import Foundation
import HTTPTypes
import MonacoAPI
import MonacoTestSupport
import XCTest

@testable import MonacoCore

final class ChatSessionSendTests: XCTestCase {
    private typealias Fixtures = ChatFixtures

    func testALiveMessageAppearsOnceAndItsEchoIsDropped() async throws {
        let mine = Fixtures.message("m1", author: Fixtures.viewerID, body: "gm", minutes: 1)
        let transport = StubTransport(scripted: [try Fixtures.page([]), try Fixtures.created(mine)])
        let session = Fixtures.session(transport)
        await session.open()

        await session.send(body: "gm")
        await session.apply(.messageCreated(mine))

        let state = await Fixtures.state(session)
        XCTAssertEqual(Fixtures.ids(state), ["m1"])
        XCTAssertEqual(state.timeline.rows.first?.delivery, .sent)
    }

    func testAnEchoThatArrivesBeforeThe201ShowsOneRow() async throws {
        let mine = Fixtures.message("m1", author: Fixtures.viewerID, body: "gm", minutes: 1)
        let transport = StubTransport(scripted: [try Fixtures.page([]), .gate])
        let session = Fixtures.session(transport)
        await session.open()

        let sending = Task { await session.send(body: "gm") }
        await transport.waitForRequests(2)
        let pending = await Fixtures.state(session)
        XCTAssertEqual(pending.timeline.rows.map(\.delivery), [.pending])

        await session.apply(.messageCreated(mine))
        let echoed = await Fixtures.state(session)
        XCTAssertEqual(Fixtures.ids(echoed), ["m1"])
        await transport.releaseGate(try Fixtures.created(mine))
        _ = await sending.value

        let state = await Fixtures.state(session)
        XCTAssertEqual(Fixtures.ids(state), ["m1"])
        XCTAssertEqual(state.timeline.rows.map(\.delivery), [.sent])
    }

    func testALiveMessageWithAFailedRowsBodyLeavesTheFailedRowAndItsRetry() async throws {
        let elsewhere = Fixtures.message("m1", author: Fixtures.viewerID, body: "gm", minutes: 1)
        let transport = StubTransport(scripted: [try Fixtures.page([]), .failure(URLError(.notConnectedToInternet))])
        let session = Fixtures.session(transport)
        await session.open()
        await session.send(body: "gm")

        await session.apply(.messageCreated(elsewhere))

        let state = await Fixtures.state(session)
        XCTAssertEqual(Fixtures.ids(state), ["m1", "key-1"])
        XCTAssertEqual(state.timeline.rows.map(\.delivery), [.sent, .failed])
    }

    func testACatchUpPageNeverSettlesAnUnsentRowByBody() async throws {
        let elsewhere = Fixtures.message("m1", author: Fixtures.viewerID, body: "gm", minutes: 1)
        let transport = StubTransport(scripted: [
            try Fixtures.page([Fixtures.message("m0", minutes: 0)]),
            .gate,
            try Fixtures.page([elsewhere]),
        ])
        let session = Fixtures.session(transport)
        await session.open()
        let sending = Task { await session.send(body: "gm") }
        await transport.waitForRequests(2)

        await Fixtures.initialAttach(session)
        await session.apply(.attached(resumed: false))
        let state = await Fixtures.state(session)
        XCTAssertEqual(Fixtures.ids(state), ["m0", "m1", "key-1"])
        XCTAssertEqual(state.timeline.rows.last?.delivery, .pending)
        await transport.releaseGate(
            try Fixtures.created(Fixtures.message("m2", author: Fixtures.viewerID, body: "gm", minutes: 2)))
        _ = await sending.value
    }

    func testTwoIdenticalSendsWhereOnlyOneLandsLeaveOneSentRowAndOneFailedRow() async throws {
        let stored = Fixtures.message("m1", author: Fixtures.viewerID, body: "gm", minutes: 1)
        let transport = StubTransport(scripted: [
            try Fixtures.page([]),
            .failure(URLError(.notConnectedToInternet)),
            try Fixtures.created(stored),
        ])
        let session = Fixtures.session(transport)
        await session.open()

        await session.send(body: "gm")
        await session.send(body: "gm")

        let state = await Fixtures.state(session)
        XCTAssertEqual(Fixtures.ids(state), ["m1", "key-1"])
        XCTAssertEqual(state.timeline.rows.map(\.delivery), [.sent, .failed])
    }

    func testAPendingSendKeepsTheIdempotencyKeyAsItsRowID() async throws {
        let transport = StubTransport(scripted: [try Fixtures.page([]), .gate])
        let session = Fixtures.session(transport)
        await session.open()

        let sending = Task { await session.send(body: "gm") }
        await transport.waitForRequests(2)

        let state = await Fixtures.state(session)
        XCTAssertEqual(Fixtures.ids(state), ["key-1"])
        let keys = await Fixtures.idempotencyKeys(transport)
        XCTAssertEqual(keys, ["key-1"])
        await transport.releaseGate(try Fixtures.created(Fixtures.message("m1", author: Fixtures.viewerID, body: "gm")))
        _ = await sending.value
    }

    func testAFailedSendIsRetriedWithTheSameKey() async throws {
        let stored = Fixtures.message("m1", author: Fixtures.viewerID, body: "gm", minutes: 1)
        let transport = StubTransport(scripted: [
            try Fixtures.page([]),
            .failure(URLError(.notConnectedToInternet)),
            try Fixtures.created(stored),
        ])
        let session = Fixtures.session(transport)
        await session.open()

        await session.send(body: "gm")
        let failed = await Fixtures.state(session)
        XCTAssertEqual(failed.timeline.rows.map(\.delivery), [.failed])
        XCTAssertEqual(Fixtures.ids(failed), ["key-1"])
        XCTAssertNotNil(failed.notice)

        await session.retry(key: "key-1")

        let state = await Fixtures.state(session)
        XCTAssertEqual(Fixtures.ids(state), ["m1"])
        XCTAssertEqual(state.timeline.rows.map(\.delivery), [.sent])
        let keys = await Fixtures.idempotencyKeys(transport)
        XCTAssertEqual(keys, ["key-1", "key-1"])
    }

    func testRetryIgnoresARowThatIsNotFailed() async throws {
        let transport = StubTransport(scripted: [try Fixtures.page([])])
        let session = Fixtures.session(transport)
        await session.open()

        await session.retry(key: "nothing")

        let sent = await transport.sent
        XCTAssertEqual(sent.count, 1)
    }

    func testADraftTheServerWouldRefuseNeverLeavesTheDevice() async throws {
        let transport = StubTransport(scripted: [try Fixtures.page([])])
        let session = Fixtures.session(transport)
        await session.open()

        let problem = await session.send(body: "   ")

        XCTAssertEqual(problem, .empty)
        let state = await Fixtures.state(session)
        XCTAssertTrue(state.timeline.rows.isEmpty)
        let sent = await transport.sent
        XCTAssertEqual(sent.count, 1)
    }

    func testRefusalToSendBecauseTheViewerLeftClosesTheChatAndKeepsTheMessages() async throws {
        let transport = StubTransport(scripted: [
            try Fixtures.page([Fixtures.message("m1")]),
            Fixtures.problem(403, "not_cabal_member", "You are not in this cabal."),
        ])
        let session = Fixtures.session(transport)
        await session.open()

        await session.send(body: "still here?")

        let state = await Fixtures.state(session)
        XCTAssertTrue(state.isClosed)
        XCTAssertEqual(Fixtures.ids(state), ["m1"])
        XCTAssertNil(state.notice)
    }

    func testAReloadThatReopensARefusedChatSubscribes() async throws {
        let transport = StubTransport(scripted: [
            Fixtures.problem(403, "not_cabal_member", "You are not in this cabal."),
            try Fixtures.page([Fixtures.message("m1")]),
        ])
        let realtime = FakeChatRealtime()
        let session = Fixtures.session(transport, realtime: realtime)
        await session.open()
        XCTAssertTrue(realtime.subscribed.isEmpty)

        await session.reload()

        let state = await Fixtures.state(session)
        XCTAssertFalse(state.isClosed)
        XCTAssertEqual(realtime.subscribed, [Fixtures.cabalID])
    }
}
