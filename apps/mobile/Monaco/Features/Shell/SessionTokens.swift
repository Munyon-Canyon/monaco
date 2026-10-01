import MonacoAPI

nonisolated final class SessionTokens: AccessTokenProvider, Sendable {
    private let privyToken: @Sendable () async -> String?
    private let refresh: @Sendable (String) async throws -> String?

    convenience init(auth: PrivyAuthService) {
        self.init(
            privyToken: { await auth.accessToken },
            refresh: { try await auth.refreshedAccessToken(replacing: $0) }
        )
    }

    init(
        privyToken: @escaping @Sendable () async -> String?,
        refresh: @escaping @Sendable (String) async throws -> String?
    ) {
        self.privyToken = privyToken
        self.refresh = refresh
    }

    func accessToken() async throws -> String? {
        await privyToken()
    }

    func refreshedToken(replacing stale: String) async throws -> String? {
        try await refresh(stale)
    }
}
