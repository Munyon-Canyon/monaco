#if DEBUG
import Foundation
@testable import MonacoCore
import XCTest

final class ChatSessionSampleTests: XCTestCase {
    private func session(_ scenario: ChatSampleScenario) -> ChatSession {
        ChatSession.sample(scenario) { Date(timeIntervalSince1970: 0) }
    }

    private func state(_ session: ChatSession) async -> ChatSession.State {
        for await state in await session.states() { return state }
        preconditionFailure("states ended")
    }

    func testTheDefaultThreadOpensOldestFirstAndTakesASend() async {
        let session = session(ChatSampleScenario())
        await session.open()

        let opened = await state(session)
        XCTAssertEqual(opened.timeline.rows.map(\.id), (1...8).map { "s\($0)" })
        XCTAssertFalse(opened.timeline.hasOlder)

        await session.send(body: "gm")
        let sent = await state(session)
        XCTAssertEqual(sent.timeline.rows.last?.message.body, "gm")
        XCTAssertEqual(sent.timeline.rows.last?.delivery, .sent)
    }

    func testAnEmptyCabalOpensLoadedWithNoRows() async {
        let session = session(ChatSampleScenario(startEmpty: true))
        await session.open()

        let opened = await state(session)
        XCTAssertEqual(opened.load, .loaded)
        XCTAssertTrue(opened.timeline.rows.isEmpty)
    }

    func testOfflineSendsFail() async {
        let session = session(ChatSampleScenario(failSends: true))
        await session.open()

        await session.send(body: "gm")

        let failed = await state(session)
        XCTAssertEqual(failed.timeline.rows.last?.delivery, .failed)
    }

    func testABusyThreadHasHistoryBeyondTheFirstPage() async {
        let session = session(ChatSampleScenario(busy: true))
        await session.open()

        let opened = await state(session)
        XCTAssertEqual(opened.timeline.rows.count, ChatSession.pageSize)
        XCTAssertTrue(opened.timeline.hasOlder)

        await session.loadOlder()
        let all = await state(session)
        XCTAssertEqual(all.timeline.rows.count, 68)
        XCTAssertFalse(all.timeline.hasOlder)
    }

    func testTheClosedFirstLoadRefusesOnceThenReopens() async {
        let session = session(ChatSampleScenario(closedFirstLoad: true))
        await session.open()

        let closed = await state(session)
        XCTAssertTrue(closed.isClosed)

        await session.reload()
        let reopened = await state(session)
        XCTAssertFalse(reopened.isClosed)
        XCTAssertEqual(reopened.load, .loaded)
    }

    func testACatchUpAfterTheNewestMessageFindsNothingNewAndCloseDetaches() async {
        let session = session(ChatSampleScenario())
        await session.open()
        let before = await state(session)

        await session.apply(.attached(resumed: false))
        await session.close()

        let after = await state(session)
        XCTAssertEqual(after.timeline.rows, before.timeline.rows)
    }

    func testAFlakyNetworkFailsTheFirstSendAndTheRetryLands() async {
        let session = session(ChatSampleScenario(failFirstSend: true))
        await session.open()

        await session.send(body: "gm")
        let failed = await state(session)
        let key = try? XCTUnwrap(failed.timeline.rows.last?.id)
        XCTAssertEqual(failed.timeline.rows.last?.delivery, .failed)

        await session.retry(key: key ?? "")
        let sent = await state(session)
        XCTAssertEqual(sent.timeline.rows.last?.delivery, .sent)
        XCTAssertEqual(sent.timeline.rows.filter { $0.message.body == "gm" }.count, 1)
    }

    func testASendRefusedBecauseTheViewerLeftClosesTheChatAndKeepsTheThread() async {
        let session = session(ChatSampleScenario(closedOnSend: true))
        await session.open()

        await session.send(body: "still here?")

        let closed = await state(session)
        XCTAssertTrue(closed.isClosed)
        XCTAssertEqual(closed.timeline.rows.count, 8)
    }
}
#endif
