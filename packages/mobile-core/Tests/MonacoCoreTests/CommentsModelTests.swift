import Foundation
import MonacoAPI
import MonacoTestClock
import MonacoTestSupport
import XCTest

@testable import MonacoCore

@MainActor
final class CommentsModelTests: XCTestCase {
    private typealias Thread = Components.Schemas.CommentThread
    private typealias Support = CommentsTestSupport
    private typealias Reply = StubTransport.Reply
    private let itemID = "item-1"
    private let clock = TestClock()

    func testAFirstPageFillsTheRowsAndTheViewerMayComment() async throws {
        let (model, transport, _) = try make([try Support.page(Thread.samples), try Support.detail(canComment: true)])

        await model.load()

        XCTAssertEqual(model.rows.map(\.id), ["c1", "c2", "c3", "c4"])
        XCTAssertEqual(model.phase, .loaded)
        XCTAssertTrue(model.canComment)
        let paths = await transport.sent.compactMap(\.path)
        XCTAssertEqual(paths, ["/v1/feed/item-1/comments", "/v1/feed/item-1"])
    }

    func testAViewerOutsideTheCabalCannotComment() async throws {
        let (model, _, _) = try make([try Support.page(Thread.samples), try Support.detail(canComment: false)])

        await model.load()

        XCTAssertFalse(model.canComment)
    }

    func testACanCommentReadThatFailedIsRetriedByTheNextLoadAndRefresh() async throws {
        let (model, transport, _) = try make([
            try Support.page(Thread.samples), .json(.internalServerError, "{}"),
            try Support.page(Thread.samples), try Support.detail(canComment: false),
        ])
        await model.load()
        XCTAssertTrue(model.canComment)

        await model.load()

        XCTAssertFalse(model.canComment)
        let paths = await transport.sent.compactMap(\.path)
        XCTAssertEqual(paths.suffix(1), ["/v1/feed/item-1"])
    }

    func testAnEmptyThreadIsEmpty() async throws {
        let (model, _, _) = try make([try Support.page([]), try Support.detail(canComment: true)])

        await model.load()

        XCTAssertEqual(model.phase, .empty)
    }

    func testAFirstLoadFailureIsFailedWithNothingOnScreen() async throws {
        let (model, _, _) = try make([.json(.internalServerError, "{}")])

        await model.load()

        guard case .failed = model.phase else { return XCTFail("\(model.phase)") }
        XCTAssertTrue(model.rows.isEmpty)
    }

    func testAFailedRefreshKeepsTheThreadAndRaisesANotice() async throws {
        let (model, _, _) = try make([
            try Support.page(Thread.samples), try Support.detail(canComment: true), .json(.internalServerError, "{}"),
        ])
        await model.load()

        await model.refresh()

        XCTAssertEqual(model.rows.count, 4)
        XCTAssertEqual(model.noticeTick, 1)
        guard case .failure = model.notice else { return XCTFail("\(String(describing: model.notice))") }
    }

    func testPostingSendsOneKeyedBodyThenRefreshesAndClearsTheReply() async throws {
        let posted = Components.Schemas.Comment.sample("c5", mine: true)
        let (model, transport, _) = try make([
            try Support.page(Thread.samples), try Support.detail(canComment: true),
            .json(.created, try Support.encode(posted)),
            try Support.page(Thread.samples),
        ])
        await model.load()
        model.beginReply(to: model.rows[1])

        let ok = await model.post("  nice  ")

        XCTAssertTrue(ok)
        XCTAssertNil(model.replyTarget)
        let sent = await transport.sent
        let bodies = await transport.sentBodies
        let post = try XCTUnwrap(sent.firstIndex { $0.method == .post })
        XCTAssertEqual(sent[post].path, "/v1/feed/item-1/comments")
        XCTAssertNotNil(try Support.idempotencyKey(of: sent[post]))
        let json = try XCTUnwrap(
            JSONSerialization.jsonObject(with: try XCTUnwrap(bodies[post])) as? [String: String])
        XCTAssertEqual(json, ["body": "nice", "parent_comment_id": "c2"])
        XCTAssertEqual(sent.count, 4)
    }

    func testARetryAfterATransportErrorReusesTheKey() async throws {
        let (model, transport, _) = try make([
            .failure(URLError(.networkConnectionLost)),
            .json(.created, try Support.encode(Components.Schemas.Comment.sample("c5", mine: true))),
            try Support.page([]),
        ])

        let first = await model.post("hello")
        let second = await model.post("hello")

        XCTAssertFalse(first)
        XCTAssertTrue(second)
        let posts = await transport.sent.filter { $0.method == .post }
        let keys = try posts.map { try Support.idempotencyKey(of: $0) }
        XCTAssertEqual(keys.count, 2)
        XCTAssertEqual(keys[0], keys[1])
        XCTAssertNotNil(keys[0])
    }

    func testAMembersOnlyRefusalRaisesANoticeAndKeepsTheDraftOut() async throws {
        let (model, _, _) = try make([
            try .problem(
                Components.Schemas.Problem(
                    _type: .about_colon_blank, title: "Error", status: 403, code: .commentMembersOnly,
                    message: "Only members can comment.", traceId: "4bf92f3577b34da6a3ce929d0e0e4736",
                    retryable: false))
        ])

        let ok = await model.post("hello")

        XCTAssertFalse(ok)
        guard case .failure(.problem(let problem)) = model.notice else {
            return XCTFail("\(String(describing: model.notice))")
        }
        XCTAssertEqual(problem.code, .known(.commentMembersOnly))
    }

    func testAnEmptyOrOverlongDraftSendsNothing() async throws {
        let (model, transport, _) = try make([])

        let blank = await model.post("   ")
        let long = await model.post(String(repeating: "a", count: CommentDraft.maxScalars + 1))

        XCTAssertFalse(blank)
        XCTAssertFalse(long)
        let sent = await transport.sent.count
        XCTAssertEqual(sent, 0)
    }

    func testDeletingYourCommentCallsDeleteSaysSoAndRefreshes() async throws {
        let mine = Thread(comment: .sample("c1", mine: true), replies: [])
        let (model, transport, _) = try make([
            try Support.page([mine]), try Support.detail(canComment: true), .json(.noContent, ""),
            try Support.page([Thread(comment: .deletedSample("c1"), replies: [])]),
        ])
        await model.load()

        await model.delete(model.rows[0])

        XCTAssertEqual(model.notice, .deleted)
        XCTAssertEqual(model.rows.first?.text, "Comment deleted")
        let sent = await transport.sent
        let delete = try XCTUnwrap(sent.first { $0.method == .delete })
        XCTAssertEqual(delete.path, "/v1/feed/comments/c1")
    }

    func testSomeoneElsesCommentIsNeverDeleted() async throws {
        let (model, transport, _) = try make([try Support.page(Thread.samples), try Support.detail(canComment: true)])
        await model.load()

        await model.delete(model.rows[0])

        let sent = await transport.sent.count
        XCTAssertEqual(sent, 2)
        XCTAssertNil(model.notice)
    }

    func testAFeedHintRefetchesTheFirstPageOnce() async throws {
        let (model, transport, hints) = try make([
            try Support.page([Thread.samples[1]]), try Support.detail(canComment: true),
            try Support.page(Thread.samples),
        ])
        await model.load()
        let observer = Task { await model.observe() }
        addTeardownBlock { observer.cancel() }
        let subscribed = await Support.waitUntil { await hints.subscriberCount == 1 }
        XCTAssertTrue(subscribed)

        await hints.send(.changed(.global, what: "feed", id: "1"))

        let merged = await Support.waitUntil { model.rows.count == 4 }
        XCTAssertTrue(merged)
        let sent = await transport.sent.count
        XCTAssertEqual(sent, 3)
    }

    func testAHintForAnotherWhatFetchesNothingAndAHiddenScreenWaits() async throws {
        let (model, transport, hints) = try make([
            try Support.page([Thread.samples[1]]), try Support.detail(canComment: true),
            try Support.page(Thread.samples),
        ])
        await model.load()
        let observer = Task { await model.observe() }
        addTeardownBlock { observer.cancel() }
        _ = await Support.waitUntil { await hints.subscriberCount == 1 }

        await hints.send(.changed(.global, what: "prices_updated", id: "1"))
        model.setVisible(false)
        await hints.send(.changed(.global, what: "feed", id: "1"))
        for _ in 0..<50 { await Task.yield() }
        let whileHidden = await transport.sent.count
        XCTAssertEqual(whileHidden, 2)

        model.setVisible(true)
        let merged = await Support.waitUntil { model.rows.count == 4 }
        XCTAssertTrue(merged)
    }

    private func make(_ script: [Reply]) throws -> (CommentsModel, StubTransport, FakeHintStream) {
        let transport = StubTransport(scripted: script)
        let hints = FakeHintStream()
        let model = CommentsModel(
            source: FeedCommentsSource(itemID: itemID, api: Support.api(transport)), hints: hints, clock: clock)
        return (model, transport, hints)
    }
}
