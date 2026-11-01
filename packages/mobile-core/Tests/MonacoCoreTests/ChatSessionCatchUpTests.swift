import Foundation
import MonacoAPI
import MonacoTestSupport
import XCTest

@testable import MonacoCore

final class ChatSessionCatchUpTests: XCTestCase {
    private typealias Fixtures = ChatFixtures

    func testACatchUpPastTenFullPagesStillReachesTheNewestMessage() async throws {
        var script: [StubTransport.Reply] = [try Fixtures.page([Fixtures.message("m0", minutes: -1)])]
        var lastPage: [ChatMessage] = []
        for pageIndex in 0..<12 {
            lastPage = (0..<ChatSession.pageSize).map {
                Fixtures.message(String(format: "p%02d-%03d", pageIndex, $0), minutes: Double(pageIndex * 100 + $0))
            }
            script.append(try Fixtures.page(lastPage))
        }
        let tail = Fixtures.message("tail", minutes: 5_000)
        script.append(try Fixtures.page([tail]))
        script.append(try Fixtures.page(Array(lastPage.suffix(ChatSession.pageSize - 1)) + [tail]))
        let transport = StubTransport(scripted: script)
        let session = Fixtures.session(transport)
        await session.open()

        await session.apply(.attached(resumed: false))

        let state = await Fixtures.state(session)
        XCTAssertEqual(state.timeline.newestID, "tail")
        XCTAssertEqual(state.timeline.rows.count, 12 * ChatSession.pageSize + 2)
        let sent = await transport.sent
        XCTAssertEqual(sent[13].path, "/v1/cabals/\(Fixtures.cabalID)/messages?after=p11-049&limit=50")
    }

    func testALiveMessageDuringACatchUpDoesNotMoveThePageCursor() async throws {
        let firstPage: [ChatMessage] = (0..<ChatSession.pageSize).map {
            Fixtures.message(String(format: "a%03d", $0), minutes: Double($0))
        }
        let secondPage: [ChatMessage] = (0..<ChatSession.pageSize).map {
            Fixtures.message(String(format: "b%03d", $0), minutes: Double(100 + $0))
        }
        let live = Fixtures.message("z001", minutes: 1_000)
        let tail = Fixtures.message("c001", minutes: 200)
        let transport = StubTransport(scripted: [
            try Fixtures.page([Fixtures.message("m0", minutes: -1)]),
            try Fixtures.page(firstPage),
            .gate,
            try Fixtures.page([tail]),
            try Fixtures.page(Array(secondPage.suffix(ChatSession.pageSize - 2)) + [tail, live]),
        ])
        let session = Fixtures.session(transport)
        await session.open()

        let catching = Task { await session.apply(.attached(resumed: false)) }
        await transport.waitForRequests(3)
        await session.apply(.messageCreated(live))
        await transport.releaseGate(try Fixtures.page(secondPage))
        await catching.value

        let sent = await transport.sent
        XCTAssertEqual(sent[3].path, "/v1/cabals/\(Fixtures.cabalID)/messages?after=b049&limit=50")
        let state = await Fixtures.state(session)
        XCTAssertEqual(state.timeline.rows.count, 2 * ChatSession.pageSize + 3)
        XCTAssertEqual(state.timeline.newestID, "z001")
    }
}
