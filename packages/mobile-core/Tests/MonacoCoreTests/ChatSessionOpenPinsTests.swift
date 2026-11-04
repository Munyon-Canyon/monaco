import MonacoTestClock
import MonacoTestSupport
import XCTest

@testable import MonacoCore

@MainActor
final class ChatSessionOpenPinsTests: XCTestCase {
    private typealias Fixtures = ChatFixtures

    func testOpenLoadsPinsEvenWhenRealtimeNeverAttaches() async throws {
        let base = "/v1/cabals/\(Fixtures.cabalID)"
        let pinned = Fixtures.message("p1", minutes: 1)
        let transport = StubTransport(routes: [
            "\(base)/messages?moments=true&limit=50": [try Fixtures.page([pinned])],
            "\(base)/chat/pins": [.json(.ok, #"{"messages":[\#(try Fixtures.json(pinned))]}"#)],
        ])
        let clock = TestClock()
        let session = Fixtures.session(
            transport, realtime: FakeChatRealtime(attaches: false), clock: clock, answersPins: false)

        let opening = Task { await session.open() }
        _ = await clock.state.until { $0.pending == 1 }
        clock.advance(by: .seconds(3))
        await opening.value

        let state = await Fixtures.state(session)
        XCTAssertEqual(state.pins.pins.map(\.id), ["p1"])
    }
}
