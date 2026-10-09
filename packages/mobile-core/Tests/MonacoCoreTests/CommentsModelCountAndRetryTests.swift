import Foundation
import MonacoAPI
import MonacoTestClock
import MonacoTestSupport
import XCTest

@testable import MonacoCore

@MainActor
final class CommentsModelCountAndRetryTests: XCTestCase {
    private typealias Thread = Components.Schemas.CommentThread
    private typealias Support = CommentsTestSupport
    private typealias Reply = StubTransport.Reply
    private let itemID = "item-1"
    private let clock = TestClock()

    func testRetryingACanCommentCheckThatFailedBringsTheComposerBack() async throws {
        let (model, transport, _) = try make([
            try Support.page(Thread.samples), .json(.internalServerError, "{}"), try Support.detail(canComment: true),
        ])
        await model.load()
        XCTAssertTrue(model.canCommentFailed)

        await model.retryCanComment()

        XCTAssertTrue(model.canComment)
        XCTAssertTrue(model.canCommentKnown)
        XCTAssertFalse(model.canCommentFailed)
        let paths = await transport.sent.compactMap(\.path)
        XCTAssertEqual(paths, ["/v1/feed/item-1/comments", "/v1/feed/item-1", "/v1/feed/item-1"])
    }

    func testARetryThatFailsAgainLeavesTheCheckFailedAndRetryable() async throws {
        let (model, _, _) = try make([
            try Support.page(Thread.samples), .json(.internalServerError, "{}"), .json(.internalServerError, "{}"),
            try Support.detail(canComment: true),
        ])
        await model.load()

        await model.retryCanComment()
        XCTAssertTrue(model.canCommentFailed)
        XCTAssertFalse(model.canCommentKnown)

        await model.retryCanComment()
        XCTAssertTrue(model.canComment)
        XCTAssertFalse(model.canCommentFailed)
    }

    func testACanCommentReadCancelledMidFlightIsNotAFailure() async throws {
        let (model, transport, _) = try make([try Support.page(Thread.samples), .hang])
        let loading = Task { await model.load() }
        await transport.waitForRequest(path: "/v1/feed/item-1")

        loading.cancel()
        await loading.value

        XCTAssertFalse(model.canCommentFailed)
        XCTAssertFalse(model.canCommentKnown)
    }

    func testTheCountSkipsDeletedCommentsAndDeletedReplies() async throws {
        let threads = [
            Thread(
                comment: .deletedSample("c1"),
                replies: [.sample("c2", parent: "c1"), .deletedSample("c3", parent: "c1")]),
            Thread(comment: .sample("c4"), replies: []),
            Thread(comment: .sample("c5"), replies: [.deletedSample("c6", parent: "c5")]),
        ]
        let (model, _, _) = try make([try Support.page(threads), try Support.detail(canComment: true)])

        await model.load()

        XCTAssertEqual(model.rows.count, 6)
        XCTAssertEqual(model.commentCount, 3)
    }

    func testTheCountIsZeroWhenEveryCommentIsDeleted() async throws {
        let threads = [Thread(comment: .deletedSample("c1"), replies: [.deletedSample("c2", parent: "c1")])]
        let (model, _, _) = try make([try Support.page(threads), try Support.detail(canComment: true)])

        await model.load()

        XCTAssertEqual(model.rows.count, 2)
        XCTAssertEqual(model.commentCount, 0)
    }

    private func make(_ script: [Reply]) throws -> (CommentsModel, StubTransport, FakeHintStream) {
        let transport = StubTransport(scripted: script)
        let hints = FakeHintStream()
        let model = CommentsModel(
            source: FeedCommentsSource(itemID: itemID, api: Support.api(transport)), hints: hints, clock: clock)
        return (model, transport, hints)
    }
}
