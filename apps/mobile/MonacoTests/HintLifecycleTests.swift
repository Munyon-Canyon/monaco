import Foundation
import MonacoAPI
import Observation
import SwiftUI
import Synchronization
import Testing

import struct MonacoCore.SessionProfile

@testable import Monaco

@MainActor
struct HintLifecycleTests {
    @Test func activeWhileSignedInStartsTheStream() async {
        let hints = FakeHintSource()
        let environment = environment(hints: hints, signedIn: true)

        await environment.sceneDidChange(.active)

        #expect(hints.starts == 1)
        #expect(hints.stops == 0)
    }

    @Test func activeWhileSignedOutLeavesTheStreamStopped() async {
        let hints = FakeHintSource()
        let environment = environment(hints: hints, signedIn: false)

        await environment.sceneDidChange(.active)

        #expect(hints.starts == 0)
    }

    @Test func signingInWhileActiveStartsTheStream() async {
        let hints = FakeHintSource()
        let environment = environment(hints: hints, signedIn: true)

        await environment.sessionDidChange(scenePhase: .active)

        #expect(hints.starts == 1)
    }

    @Test func aSessionChangeWhileSignedOutOrInTheBackgroundStartsNothing() async {
        let hints = FakeHintSource()
        let signedOut = environment(hints: hints, signedIn: false)
        let signedIn = environment(hints: hints, signedIn: true)

        await signedOut.sessionDidChange(scenePhase: .active)
        await signedIn.sessionDidChange(scenePhase: .background)

        #expect(hints.starts == 0)
    }

    @Test func activeBeforeTheSessionOpensLeavesTheStreamStopped() async {
        let hints = FakeHintSource()
        let environment = environment(hints: hints, signedIn: true, sessionOpen: false)

        await environment.sceneDidChange(.active)
        await environment.sessionDidChange(scenePhase: .active)

        #expect(hints.starts == 0)
    }

    @Test func openingTheSessionWhileActiveStartsTheStream() async throws {
        let hints = FakeHintSource()
        let environment = environment(hints: hints, signedIn: true, sessionOpen: false)
        await environment.sessionDidChange(scenePhase: .active)
        #expect(hints.starts == 0)

        environment.sessionStore.profile = try SessionProfile(json: Data(Self.profileJSON.utf8))
        while hints.starts == 0 { await Task.yield() }

        #expect(hints.starts == 1)
    }

    @Test func backgroundStopsTheStream() async {
        let hints = FakeHintSource()
        let environment = environment(hints: hints, signedIn: true)

        await environment.sceneDidChange(.background)

        #expect(hints.stops == 1)
    }

    @Test func signOutStopsTheStream() async {
        let hints = FakeHintSource()
        let environment = environment(hints: hints, signedIn: true)
        environment.viewer = Viewer(userID: "u-1", handle: nil)
        environment.navigator.selectedTab = .cabals
        environment.navigator.open(HintProbeRoute(), in: .cabals)

        await environment.signOut()

        #expect(hints.stops == 1)
        #expect(environment.viewer == nil)
        #expect(environment.navigator.selectedTab == .home)
        #expect(environment.navigator.cabalsPath.isEmpty)
        #expect(environment.navigator.homePath.isEmpty)
    }

    @Test func authSessionEndClearsTheDevSession() async {
        let hints = FakeHintSource()
        let auth = PrivyAuthService.processInstance ?? PrivyAuthService()
        let environment = AppEnvironment(
            auth: auth,
            hints: hints,
            isAuthenticated: { false },
            endAuthSession: {}
        )
        await environment.signIn(dev: DevSession(token: "dev-token", userID: "u-1"))

        auth.onSessionEnded?()

        #expect(!environment.isSignedIn)
        #expect(environment.viewer == nil)
    }

    @Test(.timeLimit(.minutes(1)))
    func aRejectedTokenThatCannotRefreshSignsOut() async {
        let hints = FakeHintSource()
        let tokens = SessionTokens(
            privyToken: { "stale-token" },
            refresh: { _ in nil }
        )
        let auth = PrivyAuthService.processInstance ?? PrivyAuthService()
        let environment = AppEnvironment(
            auth: auth,
            tokens: tokens,
            hints: hints,
            isAuthenticated: { true },
            endAuthSession: {}
        )
        environment.viewer = Viewer(userID: "u-1", handle: nil)

        _ = try? await tokens.refreshedToken(replacing: "stale-token")
        await untilViewerClears(environment)

        #expect(hints.stops == 1)
        #expect(environment.viewer == nil)
    }

    private static let profileJSON = """
        {"id":"01890a5d-ac96-774b-bcce-b302099a8058","handle":"kai","display_name":"Kai Cenat",\
        "auth_state":"ONBOARDING_COMPLETED","account_status":"active",\
        "member_wallet_address":"wallet-1","phone_linked":true,"created_at":"2026-09-30T12:00:00Z"}
        """

    private func untilViewerClears(_ environment: AppEnvironment) async {
        while environment.viewer != nil {
            await withCheckedContinuation { continuation in
                withObservationTracking {
                    _ = environment.viewer
                } onChange: {
                    continuation.resume()
                }
            }
        }
    }

    private func environment(hints: FakeHintSource, signedIn: Bool, sessionOpen: Bool = true) -> AppEnvironment {
        let auth = PrivyAuthService.processInstance ?? PrivyAuthService()
        let environment = AppEnvironment(
            auth: auth,
            hints: hints,
            isAuthenticated: { signedIn },
            endAuthSession: {}
        )
        if sessionOpen { environment.viewer = Viewer(userID: "u-1", handle: nil) }
        return environment
    }
}

private nonisolated struct HintProbeRoute: AppRoute {
    func destination() -> some View { EmptyView() }
}

nonisolated final class FakeHintSource: HintConnecting, Sendable {
    private let counts = Mutex<(starts: Int, stops: Int)>((0, 0))

    var starts: Int {
        counts.withLock { $0.starts }
    }

    var stops: Int {
        counts.withLock { $0.stops }
    }

    func hints(matching filter: HintFilter) -> AsyncStream<Hint> {
        AsyncStream { continuation in
            continuation.finish()
        }
    }

    func start() async {
        counts.withLock { $0.starts += 1 }
    }

    func stop() async {
        counts.withLock { $0.stops += 1 }
    }
}
