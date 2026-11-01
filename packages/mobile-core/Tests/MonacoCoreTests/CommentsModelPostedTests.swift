import Foundation
import MonacoAPI
import MonacoTestClock
import MonacoTestSupport
import XCTest

@testable import MonacoCore

@MainActor
final class CommentsModelPostedTests: XCTestCase {
    private typealias Thread = Components.Schemas.CommentThread
    private typealias Support = CommentsTestSupport
    private typealias Reply = StubTransport.Reply

    func testAPostedCommentShowsAtTheEndEvenWhenMorePagesWereUnloaded() async throws {
        let posted = Components.Schemas.Comment.sample("c5", mine: true)
        let created = Thread(comment: posted, replies: [])
        let (model, transport, _) = try make([
            try Support.page(Array(Thread.samples.prefix(1)), nextCursor: "p2"), try Support.detail(canComment: true),
            .json(.created, try Support.encode(posted)),
            try Support.page([Thread.samples[1], created]),
            try Support.page(Array(Thread.samples.prefix(1)), nextCursor: "p2"),
        ])
        await model.load()
        XCTAssertTrue(model.hasMore)

        let ok = await model.post(CommentDraft(text: "hello"))

        XCTAssertTrue(ok)
        XCTAssertEqual(model.rows.map(\.id), ["c1", "c2", "c3", "c4", "c5"])
        XCTAssertFalse(model.hasMore)
        XCTAssertEqual(model.lastPostedID, "c5")
        let paths = await transport.sent.compactMap(\.path)
        XCTAssertEqual(paths[3], "/v1/feed/item-1/comments?cursor=p2")
    }

    func testAPostedCommentIsAddedOnceWhenTheLastPageHasNotYetCaughtUp() async throws {
        let posted = Components.Schemas.Comment.sample("c5", mine: true)
        let (model, _, _) = try make([
            try Support.page(Array(Thread.samples.prefix(1)), nextCursor: "p2"), try Support.detail(canComment: true),
            .json(.created, try Support.encode(posted)),
            try Support.page([Thread.samples[1]]),
            try Support.page(Array(Thread.samples.prefix(1)), nextCursor: "p2"),
        ])
        await model.load()

        _ = await model.post(CommentDraft(text: "hello"))

        XCTAssertEqual(model.rows.map(\.id).filter { $0 == "c5" }.count, 1)
        XCTAssertEqual(model.rows.map(\.id).last, "c5")
    }

    private func make(_ script: [Reply]) throws -> (CommentsModel, StubTransport, FakeHintStream) {
        let transport = StubTransport(scripted: script)
        let hints = FakeHintStream()
        let model = CommentsModel(
            source: FeedCommentsSource(itemID: "item-1", api: Support.api(transport)), hints: hints, clock: TestClock())
        return (model, transport, hints)
    }
}
