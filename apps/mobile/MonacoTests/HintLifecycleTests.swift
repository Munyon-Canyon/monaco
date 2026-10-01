import MonacoAPI
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

        await environment.signOut()

        #expect(hints.stops == 1)
        #expect(environment.viewer == nil)
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
