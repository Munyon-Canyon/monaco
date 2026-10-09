import MonacoAPI
import MonacoTestClock
import MonacoTestSupport
import XCTest

@testable import MonacoCore

@MainActor
final class CommentsModelLongThreadTests: XCTestCase {
    private typealias Thread = Components.Schemas.CommentThread
    private typealias Support = CommentsTestSupport
    private typealias Reply = StubTransport.Reply

    private let pageOne = [Thread.samples[0]]
    private let pageTwo = [
        Thread(comment: .sample("c4", mine: true), replies: [.sample("c5", parent: "c4", mine: true)])
    ]

    func testATopLevelCommentOnTheLastPageAppearsAtTheEnd() async throws {
        let (model, _) = try await loaded(
            pages: 2, then: [try created("c6"), try Support.page(pageOne, nextCursor: "p2")])

        let posted = await model.post("hello")

        XCTAssertTrue(posted)
        XCTAssertEqual(model.rows.map(\.id), ["c1", "c2", "c3", "c4", "c5", "c6"])
        XCTAssertEqual(model.lastPostedID, "c6")
    }

    func testATopLevelCommentWithPagesLeftWaitsForPaging() async throws {
        let (model, _) = try await loaded(
            pages: 1, then: [try created("c6"), try Support.page(pageOne, nextCursor: "p2")])

        _ = await model.post("hello")

        XCTAssertEqual(model.rows.map(\.id), ["c1", "c2", "c3"])
    }

    func testAReplyToAThreadOnTheLastPageAppearsUnderIt() async throws {
        let (model, _) = try await loaded(
            pages: 2, then: [try created("c6", parent: "c4"), try Support.page(pageOne, nextCursor: "p2")])
        model.beginReply(to: try XCTUnwrap(model.rows.first { $0.id == "c4" }))

        _ = await model.post("agreed")

        XCTAssertEqual(model.rows.map(\.id), ["c1", "c2", "c3", "c4", "c5", "c6"])
        XCTAssertEqual(model.rows.last?.isReply, true)
    }

    func testAReplyThatARefreshAlreadyBroughtAppearsOnce() async throws {
        let withReply = Thread(
            comment: pageOne[0].comment, replies: pageOne[0].replies + [.sample("c6", parent: "c1", mine: true)])
        let refreshed = try Support.page([withReply], nextCursor: "p2")
        let failed = Reply.json(.internalServerError, "{}")
        let (model, transport) = try await loaded(pages: 2, then: [.gate, refreshed, failed])
        model.beginReply(to: try XCTUnwrap(model.rows.first { $0.id == "c1" }))
        let sending = Task { await model.post("hi") }
        _ = await Support.waitUntil { await transport.sent.count == 4 }

        await model.refresh()
        await transport.releaseGate(try created("c6", parent: "c1"))
        _ = await sending.value

        XCTAssertEqual(model.rows.map(\.id), ["c1", "c2", "c3", "c6", "c4", "c5"])
    }

    func testDeletingACommentOnTheLastPageShowsItDeleted() async throws {
        let (model, _) = try await loaded(
            pages: 2, then: [.json(.noContent, ""), try Support.page(pageOne, nextCursor: "p2")])

        await model.delete(try XCTUnwrap(model.rows.first { $0.id == "c4" }))

        let row = try XCTUnwrap(model.rows.first { $0.id == "c4" })
        XCTAssertTrue(row.isDeleted)
        XCTAssertEqual(row.text, CommentsCopy.deleted)
        XCTAssertEqual(model.rows.first { $0.id == "c5" }?.isDeleted, false)
    }

    func testDeletingAReplyOnTheLastPageShowsItDeleted() async throws {
        let (model, _) = try await loaded(
            pages: 2, then: [.json(.noContent, ""), try Support.page(pageOne, nextCursor: "p2")])

        await model.delete(try XCTUnwrap(model.rows.first { $0.id == "c5" }))

        XCTAssertEqual(model.rows.first { $0.id == "c5" }?.isDeleted, true)
        XCTAssertEqual(model.rows.first { $0.id == "c4" }?.isDeleted, false)
    }

    func testTextTypedWhileAPostSendsStaysInTheField() {
        XCTAssertEqual(CommentDraft.field("nice", afterPosting: "nice"), "")
        XCTAssertEqual(CommentDraft.field("nice \n", afterPosting: "nice"), "")
        XCTAssertEqual(CommentDraft.field("nice and more", afterPosting: "nice"), "nice and more")
        XCTAssertEqual(CommentDraft.field("", afterPosting: "nice"), "")
    }

    private func created(_ id: String, parent: String? = nil) throws -> Reply {
        .json(.created, try Support.encode(Components.Schemas.Comment.sample(id, parent: parent, mine: true)))
    }

    private func loaded(pages: Int, then script: [Reply]) async throws -> (CommentsModel, StubTransport) {
        var replies = [try Support.page(pageOne, nextCursor: "p2"), try Support.detail(canComment: true)]
        if pages == 2 { replies.append(try Support.page(pageTwo)) }
        let transport = StubTransport(scripted: replies + script)
        let model = CommentsModel(
            source: FeedCommentsSource(itemID: "item-1", api: Support.api(transport)), hints: FakeHintStream(),
            clock: TestClock())
        await model.load()
        if pages == 2 { await model.loadMore() }
        return (model, transport)
    }
}
