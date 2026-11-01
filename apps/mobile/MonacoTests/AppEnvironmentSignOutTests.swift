import Foundation
import MonacoAPI
import SwiftUI
import Testing

import enum MonacoCore.LoginFailureCopy
import struct MonacoCore.SessionAPI

@testable import Monaco

@MainActor
struct AppEnvironmentSignOutTests {
    private static let accountDeleted =
        #"{"status":403,"code":"account_deleted","message":"x","trace_id":"t","retryable":false}"#

    @Test func aRefusedRefreshEndsTheSessionAsExpired() async throws {
        var ended: [String] = []
        let tokens = SessionTokens(privyToken: { "t1" }, refresh: { _ in nil })
        let environment = environment(tokens: tokens, ended: { ended.append($0) })

        _ = try await tokens.refreshedToken(replacing: "t1")
        for _ in 0..<10_000 where ended.isEmpty { await Task.yield() }

        #expect(ended == [LoginFailureCopy.sessionExpired])
        withExtendedLifetime(environment) {}
    }

    @Test func aRejectedHintStreamTokenEndsTheSessionAsExpired() async {
        var ended: [String] = []
        let tokens = SessionTokens(privyToken: { "t1" }, refresh: { _ in nil })
        let environment = environment(tokens: tokens, ended: { ended.append($0) })

        await tokens.endSession(rejectedToken: "t1")
        for _ in 0..<10_000 where ended.isEmpty { await Task.yield() }

        #expect(ended == [LoginFailureCopy.sessionExpired])
        withExtendedLifetime(environment) {}
    }

    @Test func aMemberSignOutGivesNoReason() async {
        var ended: [String] = []
        let tokens = SessionTokens(privyToken: { "t1" }, refresh: { _ in nil })
        let environment = environment(tokens: tokens, ended: { ended.append($0) })

        await environment.signOut()

        #expect(ended == ["member"])
    }

    @Test func aDeletedAccountEndsTheSessionWithItsOwnLine() async {
        var ended: [String] = []
        let tokens = SessionTokens(privyToken: { "t1" }, refresh: { _ in nil })
        let environment = environment(tokens: tokens, ended: { ended.append($0) })

        await tokens.accountDeleted(rejectedToken: "t1")
        for _ in 0..<10_000 where ended.isEmpty { await Task.yield() }

        #expect(ended == ["This account was deleted."])
        withExtendedLifetime(environment) {}
    }

    @Test func anAccountDeletedAnswerToAnyCallEndsTheSession() async {
        var ended: [String] = []
        let tokens = SessionTokens(privyToken: { "t1" }, refresh: { _ in nil })
        let transport = StubTransport(
            .response(
                status: .forbidden, contentType: "application/problem+json", body: Data(Self.accountDeleted.utf8)
            ))
        let api = APIClient(serverURL: testServerURL, tokens: tokens, transport: transport)
        let environment = AppEnvironment(
            auth: PrivyAuthService.processInstance ?? PrivyAuthService(),
            tokens: tokens,
            api: api,
            hints: FakeHintSource(),
            isAuthenticated: { true },
            endAuthSession: { ended.append("member") },
            endExpiredSession: { ended.append($0) }
        )

        _ = try? await SessionAPI(api: api).me()
        for _ in 0..<10_000 where ended.isEmpty { await Task.yield() }

        #expect(ended == ["This account was deleted."])
        withExtendedLifetime(environment) {}
    }

    @Test func theSignedInScreensStayUntilTheAuthSessionHasEnded() async {
        var started = false
        let gate = AsyncStream.makeStream(of: Void.self)
        let hints = FakeHintSource()
        let environment = AppEnvironment(
            auth: PrivyAuthService.processInstance ?? PrivyAuthService(),
            hints: hints,
            isAuthenticated: { true },
            endAuthSession: {
                started = true
                for await _ in gate.stream { return }
            }
        )
        environment.viewer = Viewer(userID: "u-1", handle: nil)
        environment.navigator.selectedTab = .cabals
        environment.sessionStore.hasLoaded = true

        let signOut = Task { await environment.signOut() }
        for _ in 0..<10_000 where !started { await Task.yield() }

        #expect(started)
        #expect(environment.viewer != nil)
        #expect(environment.navigator.selectedTab == .cabals)
        #expect(environment.sessionStore.hasLoaded)

        await environment.sessionDidChange(scenePhase: .active)

        #expect(hints.starts == 0)

        gate.continuation.yield()
        await signOut.value

        #expect(environment.viewer == nil)
        #expect(environment.navigator.selectedTab == .home)
        #expect(environment.sessionStore.hasLoaded == false)
    }

    private func environment(tokens: SessionTokens, ended: @escaping (String) -> Void) -> AppEnvironment {
        AppEnvironment(
            auth: PrivyAuthService.processInstance ?? PrivyAuthService(),
            tokens: tokens,
            hints: FakeHintSource(),
            isAuthenticated: { true },
            endAuthSession: { ended("member") },
            endExpiredSession: { ended($0) }
        )
    }
}
