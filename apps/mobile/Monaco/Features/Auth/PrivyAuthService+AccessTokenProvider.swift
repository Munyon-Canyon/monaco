import MonacoAPI

extension PrivyAuthService: nonisolated AccessTokenProvider {
    func accessToken() async throws -> String? {
        accessToken
    }

    func refreshedToken(replacing stale: String) async throws -> String? {
        try await refreshedAccessToken(replacing: stale)
    }
}
