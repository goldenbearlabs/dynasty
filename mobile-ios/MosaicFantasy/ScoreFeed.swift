// Connects authenticated mobile score feeds and refreshes a screen when scores change.
import Foundation

enum ScoreFeed {
    static func request(url: URL, token: String) -> URLRequest {
        var request = URLRequest(url: url)
        request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        return request
    }

    static func run(urls: [URL], token: String, onChange: @escaping @Sendable () async -> Void) async {
        guard !token.isEmpty else { return }
        await withTaskGroup(of: Void.self) { group in
            for url in Set(urls) {
                group.addTask { await watch(url: url, token: token, onChange: onChange) }
            }
        }
    }

    private static func watch(url: URL, token: String, onChange: @escaping @Sendable () async -> Void) async {
        var reconnects = 0
        while !Task.isCancelled {
            let socket = URLSession.shared.webSocketTask(with: request(url: url, token: token))
            socket.resume()
            await withTaskCancellationHandler {
                var first = true
                do {
                    while !Task.isCancelled {
                        _ = try await socket.receive()
                        if first {
                            first = false
                            if reconnects > 0 { await onChange() }
                        } else {
                            await onChange()
                        }
                    }
                } catch {}
            } onCancel: {
                socket.cancel(with: .goingAway, reason: nil)
            }
            socket.cancel(with: .goingAway, reason: nil)
            if Task.isCancelled { return }
            reconnects += 1
            let delay = UInt64(min(1 << min(reconnects, 4), 15)) * 1_000_000_000
            try? await Task.sleep(nanoseconds: delay)
        }
    }
}
