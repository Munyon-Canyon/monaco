import Foundation
import MonacoAPI

public struct SessionRevoker: Sendable {
    public static let defaultWait = Duration.seconds(5)

    private let makeClient: @Sendable (String) -> APIClient

    public init(serverURL: URL) {
        makeClient = { APIClient(serverURL: serverURL, tokens: OneTokenProvider(token: $0)) }
    }

    package init(makeClient: @escaping @Sendable (String) -> APIClient) {
        self.makeClient = makeClient
    }

    @discardableResult
    public func revoke(token: String, within wait: Duration = defaultWait) async -> Bool {
        let client = makeClient(token)
        return await withTaskGroup(of: Bool.self) { group in
            group.addTask { await Self.send(client) }
            group.addTask {
                try? await Task.sleep(for: wait)
                return false
            }
            let first = await group.next() ?? false
            group.cancelAll()
            return first
        }
    }

    private static func send(_ client: APIClient) async -> Bool {
        do {
            try await client.read { client in
                _ = try await client.revokeSessions(
                    headers: .init(idempotencyKey: UUID().uuidString),
                    body: .json(.init(scope: .thisSession))
                ).noContent
            }
            return true
        } catch {
            return false
        }
    }
}

private struct OneTokenProvider: MonacoAPI.AccessTokenProvider {
    let token: String

    func accessToken() async throws -> String? { token }
    func refreshedToken(replacing _: String) async throws -> String? { nil }
}
