// Lets a manager sign in, create an account, claim an invite, or request a password reset.
import SwiftUI

struct AuthView: View {
    @ObservedObject var session: SessionStore
    @State private var mode: Mode = .signIn
    @State private var email = ""
    @State private var password = ""
    @State private var inviteURL = ""
    @State private var busy = false
    @State private var notice: String?
    @State private var isError = true

    private enum Mode: String, CaseIterable {
        case signIn = "Sign in"
        case register = "Create account"
        case claim = "Claim invite"
    }

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 22) {
                    VStack(alignment: .leading, spacing: 8) {
                        Text(mode == .signIn ? "Welcome back" : mode == .register ? "Join Mosaic Fantasy" : "Join your franchise")
                            .font(.largeTitle.bold())
                            .foregroundStyle(MosaicPalette.ink)
                        Text(mode == .signIn ? "Sign in to see your leagues." : mode == .register
                             ? "Create an account, then verify your email to join a league."
                             : "Paste the invite link your commissioner sent you.")
                            .foregroundStyle(MosaicPalette.soft)
                    }
                    VStack(spacing: 14) {
                        if mode == .claim {
                            TextField("Invite link", text: $inviteURL)
                                .textContentType(.URL)
                                .keyboardType(.URL)
                                .textInputAutocapitalization(.never)
                                .autocorrectionDisabled()
                                .accessibilityIdentifier("inviteField")
                        }
                        TextField("Email", text: $email)
                            .textContentType(.username)
                            .keyboardType(.emailAddress)
                            .textInputAutocapitalization(.never)
                            .autocorrectionDisabled()
                            .accessibilityIdentifier("emailField")
                        SecureField("Password", text: $password)
                            .textContentType(mode == .signIn ? .password : .newPassword)
                            .accessibilityIdentifier("passwordField")
                    }
                    .textFieldStyle(.roundedBorder)
                    if let notice = notice ?? session.restoreError {
                        Label(notice, systemImage: isError ? "exclamationmark.circle" : "checkmark.circle")
                            .font(.subheadline)
                            .foregroundStyle(isError ? MosaicPalette.bad : MosaicPalette.soft)
                    }
                    Button {
                        Task { await submit() }
                    } label: {
                        if busy { ProgressView().frame(maxWidth: .infinity) }
                        else { Text(mode.rawValue).frame(maxWidth: .infinity) }
                    }
                    .buttonStyle(.borderedProminent)
                    .controlSize(.large)
                    .disabled(busy)
                    VStack(spacing: 4) {
                        Button("Already have an account? Sign in") { changeMode(.signIn) }
                            .opacity(mode == .signIn ? 0 : 1)
                            .disabled(mode == .signIn)
                        if mode != .register { Button("New here? Create an account") { changeMode(.register) } }
                        if mode != .claim { Button("Have an invite? Claim a franchise") { changeMode(.claim) } }
                        if mode == .signIn {
                            Button("Forgot your password?") { Task { await resetPassword() } }
                        }
                    }
                    .font(.subheadline)
                    .frame(maxWidth: .infinity)
                    .buttonStyle(.plain)
                    .foregroundStyle(MosaicPalette.brand)
                }
                .frame(maxWidth: 460)
                .padding(24)
                .frame(maxWidth: .infinity)
            }
            .background(MosaicPalette.ground)
            .navigationTitle(mode.rawValue)
        }
    }

    private func changeMode(_ next: Mode) {
        mode = next
        notice = nil
        password = ""
    }

    private func submit() async {
        guard !email.trimmingCharacters(in: .whitespaces).isEmpty, !password.isEmpty,
              mode != .claim || !inviteURL.trimmingCharacters(in: .whitespaces).isEmpty else {
            notice = "Complete the required fields to continue."
            isError = true
            return
        }
        busy = true
        notice = nil
        defer { busy = false }
        do {
            switch mode {
            case .signIn: try await session.signIn(email: email, password: password)
            case .register:
                let sent = try await session.register(email: email, password: password)
                if !sent { notice = "Account created. Email delivery failed; resend it from Leagues."; isError = false }
            case .claim: try await session.claim(inviteURL: inviteURL, email: email, password: password)
            }
        } catch {
            notice = error.localizedDescription
            isError = true
        }
    }

    private func resetPassword() async {
        guard !email.trimmingCharacters(in: .whitespaces).isEmpty else {
            notice = "Enter your email first."
            isError = true
            return
        }
        do {
            try await session.api.requestPasswordReset(email: email)
            notice = "If that email has an account, a reset link has been sent."
            isError = false
        } catch {
            notice = error.localizedDescription
            isError = true
        }
    }
}

#Preview("Sign in") {
    AuthView(session: PreviewFixtures.sessionStore)
}
