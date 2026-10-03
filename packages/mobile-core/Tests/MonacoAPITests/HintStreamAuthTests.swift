import MonacoTestClock
import XCTest

@testable import MonacoAPI

final class HintStreamAuthTests: XCTestCase {
    private let clock = TestClock()

    func testSecond401EndsTheRejectedBearer() async throws {
        let transport = FakeStreamTransport([.unauthorized, .unauthorized], clock: clock)
        let ended = Watched<[String]>([])
        let stream = makeStream(
            transport,
            token: { "stale" },
            refresh: { _ in "fresh" },
            endSession: { token in if let token { ended.mutate { $0.append(token) } } }
        )
        await stream.start()

        let signedOut = await eventually { await stream.state == .signedOut }
        XCTAssertTrue(signedOut)
        XCTAssertEqual(ended.current, ["fresh"])
        let quiet = await clock.state.until(within: .seconds(1)) { $0.pending == 0 }
        XCTAssertTrue(quiet, "a signed-out stream keeps no timers")
        await stream.stop()
    }

    func testANilTokenNeverConnectsOrEndsTheSession() async {
        let transport = FakeStreamTransport(clock: clock)
        let endings = Watched(0)
        let stream = makeStream(
            transport,
            token: { nil },
            endSession: { _ in endings.mutate { $0 += 1 } }
        )

        await stream.start()
        let reconnecting = await eventually { await stream.state == .reconnecting }

        XCTAssertTrue(reconnecting)
        XCTAssertTrue(transport.state.current.requests.isEmpty)
        XCTAssertEqual(endings.current, 0)
        await stream.stop()
    }

    private func makeStream(
        _ transport: FakeStreamTransport,
        token: @escaping @Sendable () async throws -> String? = { "token" },
        refresh: @escaping @Sendable (String) async throws -> String? = { _ in nil },
        endSession: @escaping @Sendable (String?) async -> Void
    ) -> HintStream {
        HintStream(
            serverURL: URL(string: "https://example.test/")!, transport: transport, token: token, refresh: refresh,
            endSession: endSession,
            clock: clock)
    }

    private func eventually(_ condition: () async -> Bool) async -> Bool {
        let deadline = ContinuousClock.now + .seconds(1)
        while ContinuousClock.now < deadline {
            if await condition() { return true }
            await Task.yield()
        }
        return false
    }
}
