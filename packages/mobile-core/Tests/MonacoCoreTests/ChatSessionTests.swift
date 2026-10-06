import Foundation
import HTTPTypes
import MonacoAPI
import MonacoTestSupport
import XCTest

@testable import MonacoCore

final class ChatSessionTests: XCTestCase {
    private typealias Fixtures = ChatFixtures

    func testOpenFetchesTheNewestPageThenSubscribes() async throws {
        let older = Fixtures.message("m1", minutes: 1)
        let newer = Fixtures.message("m2", minutes: 2)
        let transport = StubTransport(scripted: [try Fixtures.page([newer, older])])
        let realtime = FakeChatRealtime()
        let session = Fixtures.session(transport, realtime: realtime)

        await session.open()

        let state = await Fixtures.state(session)
        XCTAssertEqual(state.load, .loaded)
        XCTAssertEqual(Fixtures.ids(state), ["m1", "m2"])
        XCTAssertEqual(realtime.subscribed, [Fixtures.cabalID])
        let sent = await transport.sent
        XCTAssertEqual(sent.count, 1)
        XCTAssertEqual(sent[0].path, "/v1/cabals/\(Fixtures.cabalID)/messages?limit=50")
    }

    func testAReconnectThatLostContinuityFetchesAfterTheNewestHeldID() async throws {
        let held = Fixtures.message("m1", minutes: 1)
        let missed = Fixtures.message("m2", minutes: 2)
        let transport = StubTransport(scripted: [try Fixtures.page([held]), try Fixtures.page([missed])])
        let session = Fixtures.session(transport)
        await session.open()

        await session.apply(.attached(resumed: false))

        let state = await Fixtures.state(session)
        XCTAssertEqual(Fixtures.ids(state), ["m1", "m2"])
        let sent = await transport.sent
        XCTAssertEqual(sent[1].path, "/v1/cabals/\(Fixtures.cabalID)/messages?after=m1&limit=50")
    }

    func testAResumedAttachFetchesNothing() async throws {
        let transport = StubTransport(scripted: [try Fixtures.page([Fixtures.message("m1")])])
        let session = Fixtures.session(transport)
        await session.open()

        await session.apply(.attached(resumed: true))

        let sent = await transport.sent
        XCTAssertEqual(sent.count, 1)
    }

    func testACatchUpKeepsFetchingWhileThePagesAreFull() async throws {
        let first = (0..<ChatSession.pageSize).map {
            Fixtures.message(String(format: "a%03d", $0), minutes: Double($0))
        }
        let last = Fixtures.message("b001", minutes: 100)
        let transport = StubTransport(scripted: [
            try Fixtures.page([Fixtures.message("m0", minutes: -1)]),
            try Fixtures.page(first),
            try Fixtures.page([last]),
        ])
        let session = Fixtures.session(transport)
        await session.open()

        await session.apply(.attached(resumed: false))

        let state = await Fixtures.state(session)
        XCTAssertEqual(state.timeline.rows.count, ChatSession.pageSize + 2)
        XCTAssertEqual(state.timeline.newestID, "b001")
        let sent = await transport.sent
        XCTAssertEqual(sent[2].path, "/v1/cabals/\(Fixtures.cabalID)/messages?after=a049&limit=50")
    }

    func testADeletedMessageWithRepliesBecomesAPlaceholder() async throws {
        let parent = Fixtures.message("m1", minutes: 1, replyCount: 2)
        let transport = StubTransport(scripted: [try Fixtures.page([parent])])
        let session = Fixtures.session(transport)
        await session.open()

        await session.apply(.messageDeleted(id: "m1"))

        let state = await Fixtures.state(session)
        let row = try XCTUnwrap(state.timeline.rows.first)
        XCTAssertTrue(row.isPlaceholder)
        XCTAssertNil(row.message.body)
    }

    func testADeletedMessageWithoutRepliesLeavesTheThread() async throws {
        let transport = StubTransport(scripted: [
            try Fixtures.page([Fixtures.message("m1", minutes: 1), Fixtures.message("m2", minutes: 2)])
        ])
        let session = Fixtures.session(transport)
        await session.open()

        await session.apply(.messageDeleted(id: "m1"))

        let state = await Fixtures.state(session)
        XCTAssertEqual(Fixtures.ids(state), ["m2"])
    }

    func testAThreadUpdateChangesTheReplyCountInPlace() async throws {
        let transport = StubTransport(scripted: [try Fixtures.page([Fixtures.message("m1")])])
        let session = Fixtures.session(transport)
        await session.open()
        let lastReply = Fixtures.epoch.addingTimeInterval(60)

        await session.apply(.threadUpdated(id: "m1", replyCount: 4, lastReplyAt: lastReply))

        let state = await Fixtures.state(session)
        XCTAssertEqual(state.timeline.rows.first?.message.replyCount, 4)
        XCTAssertEqual(state.timeline.rows.first?.message.lastReplyAt, lastReply)
    }

    func testAReplyThatIsNotInTheChannelDoesNotAppearInIt() async throws {
        let transport = StubTransport(scripted: [try Fixtures.page([])])
        let session = Fixtures.session(transport)
        await session.open()

        await session.apply(.messageCreated(Fixtures.message("r1", parentID: "m1")))
        await session.apply(.messageCreated(Fixtures.message("r2", parentID: "m1", alsoInChannel: true)))

        let state = await Fixtures.state(session)
        XCTAssertEqual(Fixtures.ids(state), ["r2"])
    }

    func testEventsFromTheRealtimeStreamReachTheState() async throws {
        let transport = StubTransport(scripted: [try Fixtures.page([])])
        let realtime = FakeChatRealtime()
        let session = Fixtures.session(transport, realtime: realtime)
        await session.open()

        realtime.emit(.messageCreated(Fixtures.message("m1", minutes: 1)))

        for await state in await session.states() where !state.timeline.rows.isEmpty {
            XCTAssertEqual(Fixtures.ids(state), ["m1"])
            break
        }
    }

    func testClosingDetachesAndReopeningCatchesUpInsteadOfRefetchingTheNewestPage() async throws {
        let transport = StubTransport(scripted: [
            try Fixtures.page([Fixtures.message("m1", minutes: 1)]),
            try Fixtures.page([Fixtures.message("m2", minutes: 2)]),
        ])
        let realtime = FakeChatRealtime()
        let session = Fixtures.session(transport, realtime: realtime)
        await session.open()

        await session.close()
        await session.open()

        XCTAssertEqual(realtime.detached, [Fixtures.cabalID])
        XCTAssertEqual(realtime.subscribed, [Fixtures.cabalID, Fixtures.cabalID])
        let state = await Fixtures.state(session)
        XCTAssertEqual(Fixtures.ids(state), ["m1", "m2"])
        let sent = await transport.sent
        XCTAssertEqual(sent[1].path, "/v1/cabals/\(Fixtures.cabalID)/messages?after=m1&limit=50")
    }

    func testARefusedHistoryReadClosesTheChatAndShowsTheFailure() async throws {
        let transport = StubTransport(scripted: [
            Fixtures.problem(403, "not_cabal_member", "You are not in this cabal.")
        ])
        let realtime = FakeChatRealtime()
        let session = Fixtures.session(transport, realtime: realtime)

        await session.open()

        let state = await Fixtures.state(session)
        XCTAssertTrue(state.isClosed)
        guard case .failed = state.load else { return XCTFail("expected a failed load") }
        XCTAssertTrue(realtime.subscribed.isEmpty)
    }

    func testAFirstLoadFailureCanBeRetried() async throws {
        let transport = StubTransport(scripted: [
            Fixtures.problem(500, "internal", "Something broke."),
            try Fixtures.page([Fixtures.message("m1")]),
        ])
        let session = Fixtures.session(transport)

        await session.open()
        let failed = await Fixtures.state(session)
        guard case .failed(let error) = failed.load else { return XCTFail("expected a failed load") }
        XCTAssertEqual(ToastCopy.message(for: error), "Something broke.")

        await session.reload()

        let state = await Fixtures.state(session)
        XCTAssertEqual(state.load, .loaded)
        XCTAssertEqual(Fixtures.ids(state), ["m1"])
    }

    func testACatchUpFailureKeepsTheMessagesAndRaisesANotice() async throws {
        let transport = StubTransport(scripted: [
            try Fixtures.page([Fixtures.message("m1")]),
            Fixtures.problem(500, "internal", "Something broke."),
        ])
        let session = Fixtures.session(transport)
        await session.open()

        await session.apply(.attached(resumed: false))

        let state = await Fixtures.state(session)
        XCTAssertEqual(Fixtures.ids(state), ["m1"])
        XCTAssertEqual(state.load, .loaded)
        let notice = try XCTUnwrap(state.notice)
        XCTAssertEqual(ToastCopy.message(for: notice.error), "Something broke.")
    }

    func testLoadOlderPrependsThePageBeforeTheOldestMessage() async throws {
        let newest = (0..<ChatSession.pageSize).map {
            Fixtures.message(String(format: "n%03d", $0), minutes: Double($0 + 100))
        }
        let transport = StubTransport(scripted: [
            try Fixtures.page(newest.reversed()),
            try Fixtures.page([Fixtures.message("old", minutes: 1)]),
        ])
        let session = Fixtures.session(transport)
        await session.open()

        await session.loadOlder()

        let state = await Fixtures.state(session)
        XCTAssertEqual(state.timeline.oldestID, "old")
        XCTAssertFalse(state.timeline.hasOlder)
        let sent = await transport.sent
        XCTAssertEqual(sent[1].path, "/v1/cabals/\(Fixtures.cabalID)/messages?before=n000&limit=50")
    }

    func testAFailedLoadOlderKeepsTheMessagesAndRaisesANotice() async throws {
        let newest = (0..<ChatSession.pageSize).map {
            Fixtures.message(String(format: "n%03d", $0), minutes: Double($0 + 100))
        }
        let transport = StubTransport(scripted: [
            try Fixtures.page(newest),
            Fixtures.problem(500, "internal", "Something broke."),
        ])
        let session = Fixtures.session(transport)
        await session.open()

        await session.loadOlder()

        let state = await Fixtures.state(session)
        XCTAssertEqual(state.timeline.rows.count, ChatSession.pageSize)
        XCTAssertFalse(state.isLoadingOlder)
        XCTAssertNotNil(state.notice)
    }

    func testSeenAndDetachedEventsChangeNothing() async throws {
        let transport = StubTransport(scripted: [try Fixtures.page([Fixtures.message("m1")])])
        let session = Fixtures.session(transport)
        await session.open()
        let before = await Fixtures.state(session)

        await session.apply(.seenUpdated(messageId: "m1", count: 3))
        await session.apply(.detached)

        let after = await Fixtures.state(session)
        XCTAssertEqual(after, before)
    }

    func testAnAttachBeforeAnyPageLoadedFetchesTheNewestPage() async throws {
        let transport = StubTransport(scripted: [
            Fixtures.problem(500, "internal", "Something broke."),
            try Fixtures.page([Fixtures.message("m1")]),
        ])
        let session = Fixtures.session(transport)
        await session.open()

        await session.apply(.attached(resumed: false))

        let state = await Fixtures.state(session)
        XCTAssertEqual(state.load, .loaded)
        XCTAssertEqual(Fixtures.ids(state), ["m1"])
    }
}
