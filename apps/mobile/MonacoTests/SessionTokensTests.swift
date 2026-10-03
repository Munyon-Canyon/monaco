import Synchronization
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

    @Test func refreshedTokenReturningNilFiresTheSignedOutHandler() async throws {
        let tokens = SessionTokens(
            privyToken: { "stale-token" },
            refresh: { _ in nil }
        )
        let fired = Mutex(0)
        tokens.onSignedOut { fired.withLock { $0 += 1 } }

        let refreshed = try await tokens.refreshedToken(replacing: "stale-token")

        #expect(refreshed == nil)
        #expect(fired.withLock { $0 } == 1)
    }

    @Test func refreshedTokenReturningAFreshTokenDoesNotFireTheSignedOutHandler() async throws {
        let tokens = SessionTokens(
            privyToken: { "privy-token" },
            refresh: { _ in "fresh-token" }
        )
        let fired = Mutex(0)
        tokens.onSignedOut { fired.withLock { $0 += 1 } }

        let refreshed = try await tokens.refreshedToken(replacing: "stale-token")

        #expect(refreshed == "fresh-token")
        #expect(fired.withLock { $0 } == 0)
    }

    @Test func rejectingTheActiveDevTokenFiresTheSignedOutHandler() async throws {
        let tokens = SessionTokens(
            privyToken: { "privy-token" },
            refresh: { _ in "refreshed" }
        )
        let dev = DevSession(token: "dev-token", userID: "u-1")
        tokens.use(dev)
        let fired = Mutex(0)
        tokens.onSignedOut { fired.withLock { $0 += 1 } }

        let refreshed = try await tokens.refreshedToken(replacing: dev.token)

        #expect(refreshed == nil)
        #expect(fired.withLock { $0 } == 1)
    }

    @Test func rejectingAnOldTokenWhileADevSessionIsActiveDoesNotFireTheSignedOutHandler() async throws {
        let tokens = SessionTokens(
            privyToken: { "privy-token" },
            refresh: { _ in "refreshed" }
        )
        tokens.use(DevSession(token: "dev-token", userID: "u-1"))
        let fired = Mutex(0)
        tokens.onSignedOut { fired.withLock { $0 += 1 } }

        let refreshed = try await tokens.refreshedToken(replacing: "old")

        #expect(refreshed == nil)
        #expect(fired.withLock { $0 } == 0)
    }

    @Test func rejectingAnOldTokenThatIsNotTheCurrentOneDoesNotFireTheSignedOutHandler() async throws {
        let tokens = SessionTokens(
            privyToken: { "current" },
            refresh: { _ in nil }
        )
        let fired = Mutex(0)
        tokens.onSignedOut { fired.withLock { $0 += 1 } }

        let refreshed = try await tokens.refreshedToken(replacing: "old")

        #expect(refreshed == nil)
        #expect(fired.withLock { $0 } == 0)
    }

    @Test func endingAnOldTokenDoesNotSignOutTheCurrentSession() async {
        let tokens = SessionTokens(privyToken: { "current" }, refresh: { _ in nil })
        let fired = Mutex(0)
        tokens.onSignedOut { fired.withLock { $0 += 1 } }

        await tokens.endSession(rejectedToken: "old")

        #expect(fired.withLock { $0 } == 0)
    }
}
#endif
