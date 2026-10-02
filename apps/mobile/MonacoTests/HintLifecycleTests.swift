import MonacoAPI
import Observation
import SwiftUI
import Synchronization
import Testing

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

    private func environment(hints: FakeHintSource, signedIn: Bool) -> AppEnvironment {
        let auth = PrivyAuthService.processInstance ?? PrivyAuthService()
        return AppEnvironment(
            auth: auth,
            hints: hints,
            isAuthenticated: { signedIn },
            endAuthSession: {}
        )
    }
}

private struct HintProbeRoute: AppRoute {
    func destination() -> some View { EmptyView() }
}

private nonisolated final class FakeHintSource: HintConnecting, Sendable {
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
