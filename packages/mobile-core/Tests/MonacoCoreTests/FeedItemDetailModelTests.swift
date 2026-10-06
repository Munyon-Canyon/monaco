import MonacoAPI
import MonacoTestClock
import MonacoTestSupport
import XCTest

@testable import MonacoCore

@MainActor
final class FeedItemDetailModelTests: XCTestCase {
    private typealias Support = CommentsTestSupport

    func testLoadFillsTheHeaderItemAndTheThread() async throws {
        let (model, transport, _) = try make(comments: [Support.page(Support.Thread.samples)], items: 2)

        await model.load()

        XCTAssertEqual(model.item?.id, Components.Schemas.FeedItem.samples[0].id)
        XCTAssertEqual(model.comments.rows.count, 4)
        let paths = await transport.sent.compactMap(\.path)
        XCTAssertEqual(Set(paths), ["/v1/feed/item-1/comments", "/v1/feed/item-1"])
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
            comments: [Support.page(Support.Thread.samples), Support.page(Support.Thread.samples)], items: 3)
        await model.load()
        let observer = Task { await model.observe() }
        addTeardownBlock { observer.cancel() }
        let subscribed = await Support.waitUntil { await hints.subscriberCount == 1 }
        XCTAssertTrue(subscribed)

        await hints.send(.changed(.global, what: "feed", id: "1"))

        let refetched = await Support.waitUntil { await transport.sent.count == 5 }
        XCTAssertTrue(refetched)
    }

    private func make(
        comments: [CommentsTestSupport.Reply], items: Int = 0, itemReplies: [CommentsTestSupport.Reply]? = nil
    ) throws -> (FeedItemDetailModel, PathRoutedTransport, FakeHintStream) {
        let itemReplies = try itemReplies ?? (0..<items).map { _ in try Support.detail(canComment: true) }
        let transport = PathRoutedTransport([
            "/v1/feed/item-1/comments": comments,
            "/v1/feed/item-1": itemReplies,
        ])
        let hints = FakeHintStream()
        let model = FeedItemDetailModel(
            itemID: "item-1", api: Support.api(transport), hints: hints, clock: TestClock())
        return (model, transport, hints)
    }
}
