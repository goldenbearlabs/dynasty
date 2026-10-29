// Restores revocable mobile sessions and keeps their bearer token in the iOS Keychain.
import Foundation
import Security

protocol TokenStore {
    func read() throws -> String?
    func write(_ token: String) throws
    func delete() throws
}

struct KeychainTokenStore: TokenStore {
    private let service = "com.mosaicfantasy.mobile"
    private let account = "manager-token"

    private var query: [String: Any] {
        [kSecClass as String: kSecClassGenericPassword,
         kSecAttrService as String: service,
         kSecAttrAccount as String: account]
    }

    func read() throws -> String? {
        var request = query
        request[kSecReturnData as String] = true
        request[kSecMatchLimit as String] = kSecMatchLimitOne
        var result: CFTypeRef?
        let status = SecItemCopyMatching(request as CFDictionary, &result)
        if status == errSecItemNotFound { return nil }
        guard status == errSecSuccess, let data = result as? Data,
              let value = String(data: data, encoding: .utf8) else {
            throw APIError(message: "Could not read your secure session.", statusCode: nil)
        }
        return value
    }

    func write(_ token: String) throws {
        guard let data = token.data(using: .utf8) else { return }
        var attributes = query
        attributes[kSecValueData as String] = data
        attributes[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
        let status = SecItemAdd(attributes as CFDictionary, nil)
        if status == errSecDuplicateItem {
            let update = SecItemUpdate(query as CFDictionary, [kSecValueData as String: data] as CFDictionary)
            guard update == errSecSuccess else { throw keychainError() }
        } else if status != errSecSuccess {
            throw keychainError()
        }
    }

    func delete() throws {
        let status = SecItemDelete(query as CFDictionary)
        guard status == errSecSuccess || status == errSecItemNotFound else { throw keychainError() }
    }

    private func keychainError() -> APIError {
        APIError(message: "Could not update your secure session.", statusCode: nil)
    }
}

@MainActor
final class SessionStore: ObservableObject {
    @Published private(set) var session: SessionInfo?
    @Published private(set) var token: String?
    @Published private(set) var ready = false
    @Published private(set) var restoreError: String?

    let api: APIClient
    private let tokenStore: any TokenStore

    init(api: APIClient, tokenStore: any TokenStore = KeychainTokenStore(), previewSession: SessionInfo? = nil) {
        self.api = api
        self.tokenStore = tokenStore
        self.session = previewSession
        self.ready = previewSession != nil
        self.token = previewSession == nil ? nil : "preview-token"
    }

    func restore() async {
        guard !ready else { return }
        defer { ready = true }
        do {
            token = try tokenStore.read()
            if let token { try accept(try await api.refresh(token: token)) }
        } catch let error as APIError where error.statusCode == 401 {
            try? clear()
        } catch {
            restoreError = error.localizedDescription
        }
    }

    func refresh() async {
        guard let token else { return }
        do {
            try accept(try await api.refresh(token: token))
            restoreError = nil
        } catch let error as APIError where error.statusCode == 401 {
            try? clear()
        } catch {
            restoreError = error.localizedDescription
        }
    }

    func signIn(email: String, password: String) async throws {
        try accept(try await api.login(email: email, password: password))
    }

    func register(email: String, password: String) async throws -> Bool {
        let sent = try await api.register(email: email, password: password)
        try await signIn(email: email, password: password)
        return sent
    }

    func claim(inviteURL: String, email: String, password: String) async throws {
        try accept(try await api.claim(invite: try inviteToken(inviteURL), email: email, password: password))
    }

    func acceptInvite(_ inviteURL: String) async throws {
        guard let token else { throw APIError(message: "Sign in first.", statusCode: nil) }
        try accept(try await api.accept(invite: try inviteToken(inviteURL), token: token))
    }

    func inviteToken(_ raw: String) throws -> String {
        guard let url = URL(string: raw.trimmingCharacters(in: .whitespacesAndNewlines)),
              url.scheme == api.baseURL.scheme,
              url.host == api.baseURL.host,
              url.port == api.baseURL.port,
              url.pathComponents.count == 3,
              url.pathComponents[1] == "join",
              !url.pathComponents[2].isEmpty else {
            throw APIError(message: "Paste an invite link for this Mosaic Fantasy server.", statusCode: nil)
        }
        return url.pathComponents[2]
    }

    func signOut() async throws {
        guard let token else { try clear(); return }
        do {
            try await api.logout(token: token)
        } catch let error as APIError where error.statusCode == 401 {
            // A revoked token is already signed out on the server.
        }
        try clear()
    }

    private func accept(_ result: MobileAuth) throws {
        try tokenStore.write(result.token)
        token = result.token
        session = result.session
        restoreError = nil
    }

    private func clear() throws {
        try tokenStore.delete()
        token = nil
        session = nil
        restoreError = nil
    }
}
