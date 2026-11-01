import Foundation
import MonacoAPI
import MonacoTestClock
import MonacoTestSupport
import XCTest

@testable import MonacoCore

@MainActor
final class CommentsModelCabalHintTests: XCTestCase {
    private typealias Thread = Components.Schemas.CommentThread
    private typealias Support = CommentsTestSupport

    func testACabalFeedItemHintRefetchesTheFirstPageOnce() async throws {
        let transport = StubTransport(scripted: [
            try Support.page([Thread.samples[1]]), try Support.detail(canComment: true),
            try Support.page(Thread.samples),
        ])
        let hints = FakeHintStream()
        let model = CommentsModel(
            source: FeedCommentsSource(itemID: "item-1", api: Support.api(transport)), hints: hints,
            clock: TestClock())
        await model.load()
        let observer = Task { await model.observe() }
        addTeardownBlock { observer.cancel() }
        let subscribed = await Support.waitUntil { await hints.subscriberCount == 2 }
        XCTAssertTrue(subscribed)

        await hints.send(.changed(.cabal("cabal-1"), what: "feed_item", id: "1"))

        let merged = await Support.waitUntil { model.rows.count == 4 }
        XCTAssertTrue(merged)
        let sent = await transport.sent.count
        XCTAssertEqual(sent, 3)
    }
}
