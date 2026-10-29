import MonacoAPI
import MonacoCore
import MonacoTestClock
import XCTest

@MainActor
final class FeedPreviewTests: XCTestCase {
    func testTheItemsHarnessShowsNoPriceMovesInAnyScope() async {
        let model = FeedModel.preview(.items, clock: TestClock())
        await model.load()
        let all = model.items.map(\.id)
        XCTAssertFalse(model.items.map(\.kind).contains("price_move"))
        await model.select(.following)
        XCTAssertEqual(model.items.map(\.id), all)
    }

    func testThePnlPrivateHarnessHidesEveryPersonsReturn() async {
        let model = FeedModel.preview(.pnlPrivate, clock: TestClock())
        await model.load()
        for item in model.items {
            XCTAssertEqual(item.returnPrivate, item.actorId != nil, item.title)
            if item.returnPrivate { XCTAssertNil(item.return1mBps, item.title) }
        }
    }

    func testTheLoadMoreFailedHarnessFailsTheFirstRetryOnlyOnce() async {
        let model = FeedModel.preview(.loadMoreFailed, clock: TestClock())
        await model.load()
        let first = model.items.count
        await model.loadMore()
        XCTAssertTrue(model.loadMoreFailed)
        XCTAssertEqual(model.items.count, first)
        await model.loadMore()
        XCTAssertFalse(model.loadMoreFailed)
        XCTAssertEqual(model.items.count, first + 1)
    }

    func testTheEmptyAndFailedHarnessesShowTheirStates() async {
        let empty = FeedModel.preview(.empty, clock: TestClock())
        await empty.load()
        XCTAssertEqual(empty.phase, .empty)
        let failed = FeedModel.preview(.failed, clock: TestClock())
        await failed.load()
        guard case .failed = failed.phase else { return XCTFail("expected failure, got \(failed.phase)") }
    }

    func testTheFollowsNobodyHarnessAsksToFollowInTheFollowingScope() async {
        let model = FeedModel.preview(.followsNobody, clock: TestClock())
        await model.load()
        XCTAssertEqual(model.phase, .empty)
        await model.select(.following)
        XCTAssertEqual(model.phase, .followsNobody)
    }
}

@MainActor
final class FeedItemDetailPreviewTests: XCTestCase {
    func testTheReadyHarnessLoadsThePostAndItsDiscussion() async {
        let model = FeedItemDetailModel.preview(.ready, clock: TestClock())
        await model.load()
        XCTAssertEqual(model.item?.title, "Proposed a buy")
        XCTAssertEqual(model.comments.rows.map(\.id), ["c1", "c2"])
        XCTAssertTrue(model.comments.canComment)
    }

    func testTheEmptyHarnessHasNoComments() async {
        let model = FeedItemDetailModel.preview(.empty, clock: TestClock())
        await model.load()
        XCTAssertEqual(model.comments.phase, .empty)
    }

    func testTheUnavailableHarnessIsA404() async {
        let model = FeedItemDetailModel.preview(.unavailable, clock: TestClock())
        await model.load()
        guard case .problem(let problem) = model.itemError else {
            return XCTFail("expected a problem, got \(String(describing: model.itemError))")
        }
        XCTAssertEqual(problem.status, 404)
    }

    func testTheFailedHarnessRefusesAPost() async {
        let model = FeedItemDetailModel.preview(.failed, clock: TestClock())
        await model.load()
        let posted = await model.comments.post(CommentDraft(text: "Count me in."))
        XCTAssertFalse(posted)
    }
}
