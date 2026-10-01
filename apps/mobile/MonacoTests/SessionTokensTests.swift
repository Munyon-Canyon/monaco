import Testing

@testable import Monaco

#if DEBUG
@MainActor
struct SessionTokensTests {
    @Test func devSessionOverridesPrivyUntilCleared() async throws {
        let tokens = SessionTokens(
            privyToken: { "privy-token" },
            refresh: { _ in "refreshed" }
        )

        #expect(try await tokens.accessToken() == "privy-token")

        tokens.use(DevSession(token: "dev-token", userID: "u-1"))

        #expect(try await tokens.accessToken() == "dev-token")
        #expect(try await tokens.refreshedToken(replacing: "dev-token") == nil)

        tokens.use(nil)

        #expect(try await tokens.accessToken() == "privy-token")
    }
}
#endif
