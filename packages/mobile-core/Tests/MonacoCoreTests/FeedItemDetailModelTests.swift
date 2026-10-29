import MonacoAPI
import MonacoTestClock
import MonacoTestSupport
import XCTest

@testable import MonacoCore

@MainActor
final class FeedItemDetailModelTests: XCTestCase {
    private typealias Support = CommentsTestSupport

    func testLoadFillsTheHeaderItemAndTheThread() async throws {
        let (model, transport, _) = try make(comments: [Support.page(Support.Thread.samples)])

        await model.load()

        XCTAssertEqual(model.item?.id, Components.Schemas.FeedItem.samples[0].id)
        XCTAssertEqual(model.comments.rows.count, 4)
        let paths = await transport.sent.compactMap(\.path)
        XCTAssertEqual(Set(paths), ["/v1/feed/item-1", "/v1/feed/thread-1/comments", "/v1/feed/thread-1"])
    }

    func testTheThreadItemIdIsUsedNotThePostsOwn() async throws {
        let (model, transport, _) = try make(comments: [Support.page(Support.Thread.samples)])

        await model.load()

        let paths = await transport.sent.compactMap(\.path)
        XCTAssertTrue(paths.contains("/v1/feed/thread-1/comments"))
        XCTAssertFalse(paths.contains("/v1/feed/item-1/comments"))
    }

    func testAFailedItemReadWithNothingOnScreenKeepsTheErrorForTheHeader() async throws {
        let (model, _, _) = try make(
            comments: [Support.page(Support.Thread.samples)], itemReplies: [.json(.notFound, "{}")])

        await model.load()

        XCTAssertNil(model.item)
        XCTAssertNotNil(model.itemError)
    }

    func testAFeedHintRefetchesTheItemAndTheFirstPage() async throws {
        let (model, transport, hints) = try make(
            comments: [Support.page(Support.Thread.samples), Support.page(Support.Thread.samples)], items: 2)
        await model.load()
        let observer = Task { await model.observe() }
        addTeardownBlock { observer.cancel() }
        let subscribed = await Support.waitUntil { await hints.subscriberCount == 1 }
        XCTAssertTrue(subscribed)

        await hints.send(.changed(.global, what: "feed", id: "1"))

        let refetched = await Support.waitUntil { await transport.sent.count == 5 }
        XCTAssertTrue(refetched)
    }

    func testTheReplyControlFocusesTheDockedComposerAndPresentsNoSheet() async throws {
        let (model, _, _) = try make(comments: [Support.page(Support.Thread.samples)])
        await model.load()

        model.reply()

        XCTAssertTrue(model.comments.isDockedFocused)
        XCTAssertNil(model.comments.replySheet)
    }

    func testARowsReplyStillOpensTheSheetAndLeavesTheDockedComposer() async throws {
        let (model, _, _) = try make(comments: [Support.page(Support.Thread.samples)])
        await model.load()
        model.reply()

        let row = try XCTUnwrap(model.comments.rows.first)
        model.comments.presentReplySheet(.comment(row))

        XCTAssertEqual(model.comments.replySheet, .comment(row))
        XCTAssertFalse(model.comments.isDockedFocused)
    }

    func testAnAnchorOnTheFirstPageScrollsToItAndHighlightsIt() async throws {
        let (model, _, _) = try make(
            comments: [Support.page(Support.Thread.loganAAPLThread)], anchor: "aapl-sam")

        await model.load()

        XCTAssertEqual(model.comments.scrollRequest?.id, "aapl-sam")
        XCTAssertEqual(model.comments.highlightedID, "aapl-sam")
        XCTAssertEqual(model.anchorMissingTick, 0)
        XCTAssertNil(model.comments.replySheet)
        XCTAssertFalse(model.comments.isDockedFocused)
    }

    func testAnAnchorOnAFoldedAnswerExpandsItsRoot() async throws {
        let (model, _, _) = try make(
            comments: [Support.page(Support.Thread.loganAAPLThread)], anchor: "aapl-logan")

        await model.load()

        XCTAssertTrue(model.comments.isExpanded(rootID: "aapl-nora"))
        XCTAssertEqual(model.comments.scrollRequest?.id, "aapl-logan")
        XCTAssertEqual(model.comments.highlightedID, "aapl-logan")
    }

    func testAnAnchorOnTheSecondPagePagesForwardToIt() async throws {
        let first: [Support.Thread] = Array(Support.Thread.loganAAPLThread.prefix(2))
        let second: [Support.Thread] = Array(Support.Thread.loganAAPLThread.suffix(3))
        let (model, transport, _) = try make(
            comments: [Support.page(first, nextCursor: "page-2")], anchor: "aapl-sam",
            more: ["/v1/feed/thread-1/comments?cursor=page-2": [Support.page(second)]])

        await model.load()

        XCTAssertEqual(model.comments.scrollRequest?.id, "aapl-sam")
        let pages = await transport.sent.filter { $0.path?.hasPrefix("/v1/feed/thread-1/comments") == true }
        XCTAssertEqual(pages.count, 2)
    }

    func testAnAnchorThatIsNeverFoundStaysAtTheTopAndSaysSo() async throws {
        let (model, _, _) = try make(
            comments: [Support.page(Support.Thread.loganAAPLThread)], anchor: "gone")

        await model.load()

        XCTAssertNil(model.comments.scrollRequest)
        XCTAssertEqual(model.anchorMissingTick, 1)
        XCTAssertEqual(FeedItemDetailModel.anchorMissing, "That reply was deleted.")
    }

    func testPagingToAnAnchorStopsAfterTenPages() async throws {
        var routes: [String: [CommentsTestSupport.Reply]] = [:]
        for index in 0..<14 {
            let comment = Components.Schemas.Comment.sample(
                "p\(index)", author: "Nora", body: "Page \(index)", minutesAgo: Double(60 - index))
            let page = try Support.page([Support.Thread(comment: comment, replies: [])], nextCursor: "c\(index + 1)")
            routes["/v1/feed/thread-1/comments?cursor=c\(index)"] = [page]
        }
        let first = try Support.page(
            [Support.Thread(comment: .sample("p-first", author: "Nora", body: "First", minutesAgo: 90), replies: [])],
            nextCursor: "c0")
        let (model, transport, _) = try make(comments: [first], anchor: "missing", more: routes)

        await model.load()

        let requested = await transport.sent.filter { $0.path?.hasPrefix("/v1/feed/thread-1/comments") == true }
        XCTAssertEqual(requested.count, 1 + FeedItemDetailModel.anchorPageLimit)
        XCTAssertEqual(model.anchorMissingTick, 1)
        XCTAssertNil(model.comments.scrollRequest)
    }

    private func make(
        comments: [CommentsTestSupport.Reply], items: Int = 1, itemReplies: [CommentsTestSupport.Reply]? = nil,
        anchor: String? = nil, more: [String: [CommentsTestSupport.Reply]] = [:]
    ) throws -> (FeedItemDetailModel, PathRoutedTransport, FakeHintStream) {
        var post = Components.Schemas.FeedItem.samples[0]
        post.threadItemId = "thread-1"
        let detail = CommentsTestSupport.Reply.json(
            .ok,
            try Support.encode(Components.Schemas.FeedItemDetail(item: post, visible: true, canComment: true)))
        let itemReplies = itemReplies ?? Array(repeating: detail, count: items)
        var routes: [String: [CommentsTestSupport.Reply]] = [
            "/v1/feed/item-1": itemReplies,
            "/v1/feed/thread-1": [try Support.detail(canComment: true)],
            "/v1/feed/thread-1/comments": comments,
        ]
        routes.merge(more) { $1 }
        let transport = PathRoutedTransport(routes)
        let hints = FakeHintStream()
        let model = FeedItemDetailModel(
            itemID: "item-1", anchorCommentID: anchor, api: Support.api(transport), hints: hints,
            clock: TestClock())
        return (model, transport, hints)
    }
}
