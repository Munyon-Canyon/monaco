import Foundation
import MonacoAPI
import MonacoTestSupport
import XCTest

@testable import MonacoCore

final class ChatCommentBackedTests: XCTestCase {
    private typealias Fixtures = ChatFixtures
    private typealias Author = Components.Schemas.ChatAuthor

    private let commentID = "comment-1"

    private func card(replyCount: Int = 1) -> ChatMessage {
        var card = Fixtures.message("p1", minutes: 1, replyCount: replyCount)
        card.kind = .proposal
        card.proposalId = "proposal-1"
        return card
    }

    private func reply(_ id: String, minutes: Double, alsoInChannel: Bool = false) -> ChatMessage {
        var reply = Fixtures.message(
            id, author: Fixtures.viewerID, minutes: minutes, parentID: "p1", alsoInChannel: alsoInChannel)
        reply.commentId = commentID
        return reply
    }

    private func author(_ id: String) -> Author {
        Author(id: id, handle: nil, displayName: id, photoUrl: nil)
    }

    private func counts(_ emoji: String, _ count: Int32) -> [ChatReactionCount] {
        [ChatReactionCount(emoji: emoji, count: count)]
    }

    private func threadState(_ thread: ThreadSession) async -> ThreadSession.State {
        for await state in await thread.states() { return state }
        preconditionFailure("states ended")
    }

    private func opened(
        _ extra: [StubTransport.Reply], copy: Bool = true
    ) async throws -> (ChatSession, ThreadSession, StubTransport) {
        let channel = copy ? [card(), reply("c1", minutes: 3, alsoInChannel: true)] : [card()]
        let transport = StubTransport(
            scripted: [
                try Fixtures.page(channel),
                try Fixtures.thread(parent: card(), replies: [reply("r1", minutes: 2), reply("c1", minutes: 3)]),
            ] + extra)
        let session = Fixtures.session(transport)
        await session.open()
        let thread = await session.thread(parentId: "p1")
        await thread.open()
        return (session, thread, transport)
    }

    func testACommentBackedRowReactsThroughTheCommentAndAnOrdinaryRowDoesNot() {
        let backed = reply("r1", minutes: 2)
        let ordinary = Fixtures.message("m1")

        XCTAssertTrue(backed.isCommentBacked)
        XCTAssertEqual(backed.commentID, commentID)
        XCTAssertEqual(backed.reactionTarget(cabalID: Fixtures.cabalID), .comment(commentID))
        XCTAssertFalse(ordinary.isCommentBacked)
        XCTAssertEqual(
            ordinary.reactionTarget(cabalID: Fixtures.cabalID), .chatMessage(cabalID: Fixtures.cabalID, id: "m1"))
    }

    func testTheReportTargetIsTheCommentForABackedRowAndTheMessageOtherwise() {
        XCTAssertEqual(reply("r1", minutes: 2).reportTarget, ChatReportTarget(kind: .comment, id: commentID))
        XCTAssertEqual(Fixtures.message("m1").reportTarget, ChatReportTarget(kind: .message, id: "m1"))
    }

    func testDeletingABackedReplyCallsTheCommentRouteAndHidesTheReplyAndItsChannelCopy() async throws {
        let (session, thread, transport) = try await opened([Fixtures.noContent])

        let failure = await session.delete(messageId: "r1")

        XCTAssertNil(failure)
        let sent = await transport.sent
        XCTAssertEqual(sent[2].method, .delete)
        XCTAssertEqual(sent[2].path, "/v1/feed/comments/\(commentID)")
        let channel = await Fixtures.state(session)
        XCTAssertEqual(Fixtures.ids(channel), ["p1"])
        let replies = await threadState(thread)
        XCTAssertEqual(replies.timeline.rows.map(\.id), [])
    }

    func testDeletingTheChannelCopyAlsoHidesTheThreadReply() async throws {
        let (session, thread, transport) = try await opened([Fixtures.noContent])

        _ = await session.delete(messageId: "c1")

        let sent = await transport.sent
        XCTAssertEqual(sent[2].path, "/v1/feed/comments/\(commentID)")
        let replies = await threadState(thread)
        XCTAssertEqual(replies.timeline.rows.map(\.id), [])
    }

    func testAFailedCommentDeleteRestoresTheReplyAndItsChannelCopy() async throws {
        let (session, thread, _) = try await opened([
            Fixtures.problem(403, "comment_forbidden", "Only the author can delete it.")
        ])

        let failure = await session.delete(messageId: "r1")

        XCTAssertNotNil(failure)
        let channel = await Fixtures.state(session)
        XCTAssertEqual(Fixtures.ids(channel), ["p1", "c1"])
        let replies = await threadState(thread)
        XCTAssertEqual(replies.timeline.rows.map(\.id), ["r1", "c1"])
    }

    func testAnOrdinaryDeleteStillUsesTheChatRoute() async throws {
        let mine = Fixtures.message("m1", author: Fixtures.viewerID, minutes: 1)
        let transport = StubTransport(scripted: [try Fixtures.page([mine]), Fixtures.noContent])
        let session = Fixtures.session(transport)
        await session.open()

        _ = await session.delete(messageId: "m1")

        let sent = await transport.sent
        XCTAssertEqual(sent[1].path, "/v1/cabals/\(Fixtures.cabalID)/messages/m1")
    }

    func testAReactionForTheCommentIDUpdatesTheReplyAndTheChannelCopy() async throws {
        let (session, thread, _) = try await opened([])

        await session.apply(.reactionUpdated(messageId: commentID, reactions: counts("❤️", 2), total: 2))

        let channel = await Fixtures.state(session)
        let copy = try XCTUnwrap(channel.timeline.message(id: "c1"))
        XCTAssertEqual(copy.reactions.map(\.emoji), ["❤️"])
        XCTAssertEqual(copy.reactionCount, 2)
        let replies = await threadState(thread)
        XCTAssertEqual(replies.timeline.message(id: "r1")?.reactionCount, 2)
        XCTAssertEqual(replies.timeline.message(id: "c1")?.reactionCount, 2)
    }

    func testADeletedEventForTheCommentIDHidesTheReplyAndTheChannelCopy() async throws {
        let (session, thread, _) = try await opened([])

        await session.apply(.messageDeleted(id: commentID))

        let channel = await Fixtures.state(session)
        XCTAssertEqual(Fixtures.ids(channel), ["p1"])
        let replies = await threadState(thread)
        XCTAssertEqual(replies.timeline.rows.map(\.id), [])
    }

    func testThreadUpdatedCarriesTheLastRepliersToTheCardAndTheOpenThread() async throws {
        let (session, thread, _) = try await opened([])
        let faces = [author(Fixtures.otherID)]

        await session.apply(
            .threadUpdated(id: "p1", replyCount: 4, lastReplyAt: Fixtures.epoch, lastRepliers: faces))

        let channel = await Fixtures.state(session)
        let updated = try XCTUnwrap(channel.timeline.message(id: "p1"))
        XCTAssertEqual(updated.replyCount, 4)
        XCTAssertEqual(updated.lastReplyAt, Fixtures.epoch)
        XCTAssertEqual(updated.lastRepliers, faces)
        let replies = await threadState(thread)
        XCTAssertEqual(replies.parent?.lastRepliers, faces)
    }

    func testThreadUpdatedWithoutRepliersKeepsTheOnesTheCardHas() async throws {
        let (session, _, _) = try await opened([])
        let faces = [author(Fixtures.otherID)]
        await session.apply(
            .threadUpdated(id: "p1", replyCount: 2, lastReplyAt: Fixtures.epoch, lastRepliers: faces))

        await session.apply(.threadUpdated(id: "p1", replyCount: 3, lastReplyAt: Fixtures.epoch))

        let channel = await Fixtures.state(session)
        XCTAssertEqual(channel.timeline.message(id: "p1")?.lastRepliers, faces)
    }

    func testThreadUpdatedDropsBlockedAuthorsFromTheFaces() async throws {
        let blocks = StubTransport.Reply.json(
            .ok, #"{"users":[{"user_id":"\#(Fixtures.otherID)","handle":"h","display_name":"H","photo_url":null}]}"#)
        let transport = StubTransport(routes: [
            "/v1/me/blocks": [blocks],
            "/v1/cabals/\(Fixtures.cabalID)/messages?moments=true&limit=50": [try Fixtures.page([card()])],
        ])
        let session = Fixtures.session(transport, blocked: true)
        await session.open()

        await session.apply(
            .threadUpdated(
                id: "p1", replyCount: 2, lastReplyAt: Fixtures.epoch,
                lastRepliers: [author(Fixtures.otherID), author("friend")]))

        let channel = await Fixtures.state(session)
        XCTAssertEqual(channel.timeline.message(id: "p1")?.lastRepliers.map(\.id), ["friend"])
    }

    func testTheDecoderReadsLastRepliersFromThreadUpdated() {
        let data = Data(
            (#"{"message_id":"p1","reply_count":3,"last_reply_at":"2026-10-03T12:00:00Z","#
                + #""last_repliers":[{"id":"u1","handle":null,"display_name":"Kai","photo_url":null}]}"#).utf8)

        XCTAssertEqual(
            ChatEventDecoder.decode(name: "thread.updated", data: data),
            .threadUpdated(
                id: "p1", replyCount: 3, lastReplyAt: Date(timeIntervalSince1970: 1_791_028_800),
                lastRepliers: [Author(id: "u1", handle: nil, displayName: "Kai", photoUrl: nil)])
        )
    }

    func testABackedRowCannotBePinnedButAnOrdinaryRowCan() async {
        let transport = StubTransport(scripted: [])
        let api = APIClient(
            serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        let pins = ChatPinsModel(cabalID: Fixtures.cabalID, viewerID: Fixtures.viewerID, api: api)
        let top = Fixtures.message("m1", author: Fixtures.viewerID, minutes: 1)
        var backed = top
        backed.commentId = commentID

        let canPinBacked = await pins.canPin(backed, role: "admin", viewerID: Fixtures.viewerID)
        let canPinOrdinary = await pins.canPin(top, role: "admin", viewerID: Fixtures.viewerID)

        XCTAssertFalse(canPinBacked)
        XCTAssertTrue(canPinOrdinary)
    }

    func testAChannelCopyOfAProposalReplyNamesTheProposal() async throws {
        let (session, _, _) = try await opened([])

        let channel = await Fixtures.state(session)

        XCTAssertEqual(channel.timeline.rows.first { $0.id == "c1" }?.parentProposalID, "proposal-1")
        XCTAssertNil(channel.timeline.rows.first { $0.id == "p1" }?.parentProposalID)
    }

    func testACopyWhoseParentIsAnOrdinaryMessageHasNoParentProposal() async throws {
        let parent = Fixtures.message("m1", minutes: 1, replyCount: 1)
        let copy = Fixtures.message("m2", minutes: 2, parentID: "m1", alsoInChannel: true)
        let transport = StubTransport(scripted: [try Fixtures.page([parent, copy])])
        let session = Fixtures.session(transport)
        await session.open()

        let channel = await Fixtures.state(session)

        XCTAssertNil(channel.timeline.rows.first { $0.id == "m2" }?.parentProposalID)
    }
}
