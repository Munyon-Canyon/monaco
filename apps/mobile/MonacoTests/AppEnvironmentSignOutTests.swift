import Testing

@testable import Monaco

@MainActor
struct AppEnvironmentSignOutTests {
    @Test func aRefusedRefreshEndsTheSessionAsExpired() async throws {
        var ended: [String] = []
        let tokens = SessionTokens(privyToken: { "t1" }, refresh: { _ in nil })
        let environment = environment(tokens: tokens, ended: { ended.append($0) })

        _ = try await tokens.refreshedToken(replacing: "t1")
        for _ in 0..<10_000 where ended.isEmpty { await Task.yield() }

        #expect(ended == ["expired"])
        withExtendedLifetime(environment) {}
    }

    @Test func aRejectedHintStreamTokenEndsTheSessionAsExpired() async {
        var ended: [String] = []
        let tokens = SessionTokens(privyToken: { "t1" }, refresh: { _ in nil })
        let environment = environment(tokens: tokens, ended: { ended.append($0) })

        await tokens.endSession(rejectedToken: "t1")
        for _ in 0..<10_000 where ended.isEmpty { await Task.yield() }

        #expect(ended == ["expired"])
        withExtendedLifetime(environment) {}
    }

    @Test func aMemberSignOutGivesNoReason() async {
        var ended: [String] = []
        let tokens = SessionTokens(privyToken: { "t1" }, refresh: { _ in nil })
        let environment = environment(tokens: tokens, ended: { ended.append($0) })

        await environment.signOut()

        #expect(ended == ["member"])
    }

    private func environment(tokens: SessionTokens, ended: @escaping (String) -> Void) -> AppEnvironment {
        AppEnvironment(
            auth: PrivyAuthService.processInstance ?? PrivyAuthService(),
            tokens: tokens,
            hints: FakeHintSource(),
            isAuthenticated: { true },
            endAuthSession: { ended("member") },
            endExpiredSession: { ended("expired") }
        )
    }
}
