import Foundation
import MonacoAPI
import MonacoTestSupport
import XCTest

@testable import MonacoCore

final class ThreadSessionTests: XCTestCase {
    private typealias Fixtures = ChatFixtures

    private let parent = Fixtures.message("p1", minutes: 1, replyCount: 2)

    private func reply(_ id: String, minutes: Double, of parentID: String = "p1", mine: Bool = false) -> ChatMessage {
        Fixtures.message(
            id, author: mine ? Fixtures.viewerID : Fixtures.otherID, minutes: minutes, parentID: parentID)
    }

    private func ids(_ state: ThreadSession.State) -> [String] { state.timeline.rows.map(\.id) }

    private func state(_ thread: ThreadSession) async -> ThreadSession.State {
        for await state in await thread.states() { return state }
        preconditionFailure("states ended")
    }

    func testOpenLoadsTheParentAndRepliesOldestFirst() async throws {
        let transport = StubTransport(scripted: [
            try Fixtures.thread(parent: parent, replies: [reply("r1", minutes: 2), reply("r2", minutes: 3)])
        ])
        let realtime = FakeChatRealtime()
        let thread = await Fixtures.session(transport, realtime: realtime).thread(parentId: "p1")

        await thread.open()

        let loaded = await state(thread)
        XCTAssertEqual(loaded.load, .loaded)
        XCTAssertEqual(loaded.parent?.id, "p1")
        XCTAssertEqual(ids(loaded), ["r1", "r2"])
        XCTAssertEqual(realtime.subscribed, [Fixtures.cabalID])
        let sent = await transport.sent
        XCTAssertEqual(sent[0].path, "/v1/cabals/\(Fixtures.cabalID)/messages/p1/thread?limit=50")
    }

    func testALiveReplyIsAppendedAndAReplyForAnotherParentIsIgnored() async throws {
        let transport = StubTransport(scripted: [try Fixtures.thread(parent: parent, replies: [])])
        let session = Fixtures.session(transport)
        let thread = await session.thread(parentId: "p1")
        await thread.open()

        await session.apply(.messageCreated(reply("r1", minutes: 2)))
        await session.apply(.messageCreated(reply("x1", minutes: 3, of: "p2")))

        let current = await state(thread)
        XCTAssertEqual(ids(current), ["r1"])
    }

    func testAReplyDeliveredTwiceAppearsOnce() async throws {
        let existing = reply("r1", minutes: 2)
        let transport = StubTransport(scripted: [try Fixtures.thread(parent: parent, replies: [existing])])
        let session = Fixtures.session(transport)
        let thread = await session.thread(parentId: "p1")
        await thread.open()

        await session.apply(.messageCreated(existing))
        await session.apply(.messageCreated(reply("r2", minutes: 3)))
        await session.apply(.messageCreated(reply("r2", minutes: 3)))

        let current = await state(thread)
        XCTAssertEqual(ids(current), ["r1", "r2"])
    }

    func testAThreadUpdateChangesTheParentCountsAndAnotherParentsUpdateDoesNot() async throws {
        let transport = StubTransport(scripted: [try Fixtures.thread(parent: parent, replies: [])])
        let session = Fixtures.session(transport)
        let thread = await session.thread(parentId: "p1")
        await thread.open()
        let lastReply = Fixtures.epoch.addingTimeInterval(600)

        await session.apply(.threadUpdated(id: "p2", replyCount: 9, lastReplyAt: lastReply))
        await session.apply(.threadUpdated(id: "p1", replyCount: 5, lastReplyAt: lastReply))

        let current = await state(thread)
        XCTAssertEqual(current.parent?.replyCount, 5)
        XCTAssertEqual(current.parent?.lastReplyAt, lastReply)
    }

    func testLoadOlderFetchesTheRepliesBeforeTheOldestOne() async throws {
        let full = (0..<ChatSession.pageSize).map { reply(String(format: "r%03d", $0 + 1), minutes: Double($0 + 10)) }
        let transport = StubTransport(scripted: [
            try Fixtures.thread(parent: parent, replies: full),
            try Fixtures.thread(parent: parent, replies: [reply("r000", minutes: 5)]),
        ])
        let thread = await Fixtures.session(transport).thread(parentId: "p1")
        await thread.open()

        await thread.loadOlder()

        let current = await state(thread)
        XCTAssertEqual(ids(current).first, "r000")
        XCTAssertFalse(current.timeline.hasOlder)
        let sent = await transport.sent
        XCTAssertEqual(sent[1].path, "/v1/cabals/\(Fixtures.cabalID)/messages/p1/thread?before=r001&limit=50")
    }

    func testAReplySendsTheParentAndTheChannelFlag() async throws {
        let stored = reply("r1", minutes: 2, mine: true)
        let transport = StubTransport(scripted: [
            try Fixtures.thread(parent: parent, replies: []),
            try Fixtures.created(stored),
            try Fixtures.thread(parent: parent, replies: [stored]),
        ])
        let thread = await Fixtures.session(transport).thread(parentId: "p1")
        await thread.open()

        await thread.send(body: "agree", alsoInChannel: true)

        let bodies = await transport.sentBodies
        let body = try XCTUnwrap(bodies[1])
        let json = try XCTUnwrap(JSONSerialization.jsonObject(with: body) as? [String: Any])
        XCTAssertEqual(json["body"] as? String, "agree")
        XCTAssertEqual(json["parent_id"] as? String, "p1")
        XCTAssertEqual(json["also_in_channel"] as? Bool, true)
        let current = await state(thread)
        XCTAssertEqual(ids(current), ["r1"])
        XCTAssertEqual(current.timeline.rows.map(\.delivery), [.sent])
    }

    func testAnAlsoInChannelReplyShowsInTheChannelToo() async throws {
        let stored = Fixtures.message(
            "r1", author: Fixtures.viewerID, minutes: 2, parentID: "p1", alsoInChannel: true)
        let transport = StubTransport(scripted: [
            try Fixtures.page([parent]),
            try Fixtures.created(stored),
            try Fixtures.thread(parent: Fixtures.message("p1", minutes: 1, replyCount: 3), replies: [stored]),
        ])
        let session = Fixtures.session(transport)
        await session.open()

        await session.send(body: "ship it", parentId: "p1", alsoInChannel: true)

        let channel = await Fixtures.state(session)
        XCTAssertEqual(Fixtures.ids(channel), ["p1", "r1"])
        XCTAssertEqual(channel.timeline.rows.first?.message.replyCount, 3)
        XCTAssertEqual(channel.timeline.rows.last?.parentBody, parent.body)
    }

    func testAFailedReplyIsRetriedOnceWithTheSameKey() async throws {
        let stored = reply("r1", minutes: 2, mine: true)
        let transport = StubTransport(scripted: [
            try Fixtures.thread(parent: parent, replies: []),
            .failure(URLError(.notConnectedToInternet)),
            try Fixtures.created(stored),
            try Fixtures.thread(parent: parent, replies: [stored]),
        ])
        let thread = await Fixtures.session(transport).thread(parentId: "p1")
        await thread.open()

        await thread.send(body: "agree")
        let failed = await state(thread)
        XCTAssertEqual(failed.timeline.rows.map(\.delivery), [.failed])
        XCTAssertNotNil(failed.notice)

        await thread.retry(key: "key-1")

        let settled = await state(thread)
        XCTAssertEqual(ids(settled), ["r1"])
        let keys = await Fixtures.idempotencyKeys(transport)
        XCTAssertEqual(keys, ["key-1", "key-1"])
    }

    func testAFirstLoadFailureCanBeRetried() async throws {
        let transport = StubTransport(scripted: [
            Fixtures.problem(500, "internal", "Something went wrong."),
            try Fixtures.thread(parent: parent, replies: []),
        ])
        let thread = await Fixtures.session(transport).thread(parentId: "p1")

        await thread.open()
        let failed = await state(thread)
        guard case .failed = failed.load else { return XCTFail("expected a failed load, got \(failed.load)") }
        XCTAssertNil(failed.parent)

        await thread.reload()
        let loaded = await state(thread)
        XCTAssertEqual(loaded.load, .loaded)
        XCTAssertEqual(loaded.parent?.id, "p1")
    }

    func testARefusedLoadClosesTheThread() async throws {
        let transport = StubTransport(scripted: [
            Fixtures.problem(403, "not_cabal_member", "You are not in this cabal.")
        ])
        let thread = await Fixtures.session(transport).thread(parentId: "p1")

        await thread.open()

        let current = await state(thread)
        XCTAssertTrue(current.isClosed)
    }

    func testARefusedSendClosesTheThreadAndDropsThePendingReply() async throws {
        let transport = StubTransport(scripted: [
            try Fixtures.thread(parent: parent, replies: []),
            Fixtures.problem(403, "not_cabal_member", "You are not in this cabal."),
        ])
        let thread = await Fixtures.session(transport).thread(parentId: "p1")
        await thread.open()

        await thread.send(body: "agree")

        let current = await state(thread)
        XCTAssertTrue(current.isClosed)
        XCTAssertEqual(ids(current), [])
    }

    func testTheChannelKeepsListeningWhileAThreadIsOpen() async throws {
        let transport = StubTransport(scripted: [
            try Fixtures.page([parent]), try Fixtures.thread(parent: parent, replies: []),
        ])
        let realtime = FakeChatRealtime()
        let session = Fixtures.session(transport, realtime: realtime)
        await session.open()
        let thread = await session.thread(parentId: "p1")
        await thread.open()

        await session.close()
        XCTAssertEqual(realtime.detached, [])

        await thread.close()
        XCTAssertEqual(realtime.detached, [Fixtures.cabalID])
        XCTAssertEqual(realtime.subscribed, [Fixtures.cabalID])
    }
}
