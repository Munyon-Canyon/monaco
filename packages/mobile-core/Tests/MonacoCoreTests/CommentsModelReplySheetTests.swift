import Foundation
import MonacoAPI
import MonacoTestClock
import MonacoTestSupport
import XCTest

@testable import MonacoCore

@MainActor
final class CommentsModelReplySheetTests: XCTestCase {
    private typealias Thread = Components.Schemas.CommentThread
    private typealias Support = CommentsTestSupport
    private let itemID = "item-1"
    private let clock = TestClock()

    func testFocusingTheDockedBarRaisesTheKeyboardAndOpensNoSheet() async throws {
        let (model, _) = try make([])

        model.focusDocked()

        XCTAssertTrue(model.isDockedFocused)
        XCTAssertNil(model.replySheet)
    }

    func testFocusingTheDockedBarClosesASheetThatWasOpen() async throws {
        let (model, _) = try make([])
        model.presentReplySheet(.thread)

        model.focusDocked()

        XCTAssertNil(model.replySheet)
        XCTAssertTrue(model.isDockedFocused)
    }

    func testPresentingTheThreadSheetOpensItWithNoQuoteAndUnfocusesTheBar() async throws {
        let (model, _) = try make([])
        model.focusDocked()

        model.presentReplySheet(.thread)

        XCTAssertEqual(model.replySheet, .thread)
        XCTAssertFalse(model.isDockedFocused)
    }

    func testPresentingACommentSheetOpensItAndUnfocusesTheBar() async throws {
        let (model, _) = try await loaded()
        let row = model.rows[1]
        model.focusDocked()

        model.presentReplySheet(.comment(row))

        XCTAssertEqual(model.replySheet, .comment(row))
        XCTAssertFalse(model.isDockedFocused)
    }

    func testADeletedCommentCannotBeRepliedTo() async throws {
        let gone = Thread(comment: .deletedSample("c1"), replies: [])
        let (model, _) = try make([try Support.page([gone]), try Support.detail(canComment: true)])
        await model.load()

        model.presentReplySheet(.comment(model.rows[0]))

        XCTAssertNil(model.replySheet)
    }

    func testAReplyFromACommentPostsWithThatCommentsId() async throws {
        let (model, transport) = try await loaded(posting: true)
        model.presentReplySheet(.comment(model.rows[2]))

        let ok = await model.post(CommentDraft(text: "agreed"))

        XCTAssertTrue(ok)
        XCTAssertNil(model.replySheet)
        let json = try await postedJSON(transport)
        XCTAssertEqual(json["parent_comment_id"], "c3")
        XCTAssertEqual(json["body"], "agreed")
    }

    func testAReplyFromTheThreadPostsTopLevel() async throws {
        let (model, transport) = try await loaded(posting: true)
        model.presentReplySheet(.thread)

        let ok = await model.post(CommentDraft(text: "agreed"))

        XCTAssertTrue(ok)
        let json = try await postedJSON(transport)
        XCTAssertNil(json["parent_comment_id"])
    }

    func testAPostFromTheDockedBarPostsTopLevel() async throws {
        let (model, transport) = try await loaded(posting: true)
        model.focusDocked()

        let ok = await model.post(CommentDraft(text: "agreed"))

        XCTAssertTrue(ok)
        let json = try await postedJSON(transport)
        XCTAssertNil(json["parent_comment_id"])
    }

    func testAReplyFromAFeedPostPostsTopLevelCallsOnPostedAndLoadsNoThread() async throws {
        let item = Components.Schemas.FeedItem.samples[0]
        let transport = StubTransport(scripted: [
            .json(.created, try Support.encode(Components.Schemas.Comment.sample("c9", mine: true)))
        ])
        var posted = 0
        let model = CommentsModel.forPost(
            item: item, api: Support.api(transport), hints: FakeHintStream(), clock: clock, onPosted: { posted += 1 })
        model.presentReplySheet(.post(item))

        let ok = await model.post(CommentDraft(text: "Clean exit."))

        XCTAssertTrue(ok)
        XCTAssertEqual(posted, 1)
        XCTAssertNil(model.replySheet)
        let sent = await transport.sent
        XCTAssertEqual(sent.compactMap(\.path), ["/v1/feed/\(item.threadItemId)/comments"])
        let bodies = await transport.sentBodies
        let json = try XCTUnwrap(
            JSONSerialization.jsonObject(with: try XCTUnwrap(bodies[0])) as? [String: String])
        XCTAssertEqual(json, ["body": "Clean exit."])
    }

    func testAFailedSendKeepsTheSheetAndTheDraft() async throws {
        let (model, _) = try make([.failure(URLError(.networkConnectionLost))])
        model.presentReplySheet(.thread)
        model.edit("Count me in.")

        let ok = await model.post(CommentDraft(text: model.draft))

        XCTAssertFalse(ok)
        XCTAssertEqual(model.replySheet, .thread)
        XCTAssertEqual(model.draft, "Count me in.")
        XCTAssertTrue(model.sendFailed)
        model.edit("Count me in. Please.")
        XCTAssertFalse(model.sendFailed)
    }

    func testTheDraftSurvivesClosingAndReopeningTheSheetAndIsClearedByASend() async throws {
        let (model, _) = try await loaded(posting: true)
        model.presentReplySheet(.thread)
        model.edit("Fine by me if we cap it at 45%")

        model.dismissReplySheet()
        model.presentReplySheet(.thread)

        XCTAssertEqual(model.draft, "Fine by me if we cap it at 45%")
        let ok = await model.post(CommentDraft(text: model.draft))
        XCTAssertTrue(ok)
        XCTAssertEqual(model.draft, "")
    }

    func testDeletingTheCommentBeingRepliedToClosesTheSheet() async throws {
        let mine = Thread(comment: .sample("c1", mine: true), replies: [])
        let (model, _) = try make([
            try Support.page([mine]), try Support.detail(canComment: true), .json(.noContent, ""),
            try Support.page([Thread(comment: .deletedSample("c1"), replies: [])]),
        ])
        await model.load()
        model.presentReplySheet(.comment(model.rows[0]))

        await model.delete(model.rows[0])

        XCTAssertNil(model.replySheet)
    }

    private func loaded(posting: Bool = false) async throws -> (CommentsModel, StubTransport) {
        var script = [try Support.page(Thread.samples), try Support.detail(canComment: true)]
        if posting {
            script += [
                .json(.created, try Support.encode(Components.Schemas.Comment.sample("c5", mine: true))),
                try Support.page(Thread.samples),
            ]
        }
        let (model, transport) = try make(script)
        await model.load()
        return (model, transport)
    }

    private func postedJSON(_ transport: StubTransport) async throws -> [String: String] {
        let sent = await transport.sent
        let bodies = await transport.sentBodies
        let post = try XCTUnwrap(sent.firstIndex { $0.method == .post })
        return try XCTUnwrap(JSONSerialization.jsonObject(with: try XCTUnwrap(bodies[post])) as? [String: String])
    }

    private func make(_ script: [StubTransport.Reply]) throws -> (CommentsModel, StubTransport) {
        let transport = StubTransport(scripted: script)
        let model = CommentsModel(
            source: FeedCommentsSource(itemID: itemID, api: Support.api(transport)), hints: FakeHintStream(),
            clock: clock)
        return (model, transport)
    }
}
