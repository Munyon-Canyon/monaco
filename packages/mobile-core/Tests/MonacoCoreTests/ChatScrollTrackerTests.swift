import Foundation
import MonacoCore
import XCTest

final class ChatScrollTrackerTests: XCTestCase {
    private typealias Fixtures = ChatFixtures

    private func arrival(mine: Bool = false, count: Int = 1) -> [ChatRow] {
        var timeline = ChatTimeline(viewerID: Fixtures.viewerID)
        let author = mine ? Fixtures.viewerID : Fixtures.otherID
        timeline.mergeNewest((0..<count).map { Fixtures.message("m\($0)", author: author) }, pageSize: 50)
        return timeline.rows
    }

    private func scrolledUp() -> ChatScrollTracker {
        var tracker = ChatScrollTracker()
        tracker.positionChanged(.init(offset: 500, isAtEnd: false))
        tracker.dragBegan()
        tracker.positionChanged(.init(offset: 100, isAtEnd: false))
        tracker.scrollSettled()
        return tracker
    }

    func testAThreadAtTheEndFollowsAnArrival() {
        var tracker = ChatScrollTracker()

        XCTAssertTrue(tracker.arrived(arrival()))
        XCTAssertEqual(tracker.unreadCount, 0)
    }

    func testAReaderScrolledUpIsToldAboutArrivalsInsteadOfBeingMoved() {
        var tracker = scrolledUp()

        XCTAssertFalse(tracker.arrived(arrival(count: 2)))
        XCTAssertEqual(tracker.unreadCount, 2)
        XCTAssertTrue(tracker.readerControlsScroll)
    }

    func testTheViewersOwnMessageAlwaysFollowsAndClearsTheCount() {
        var tracker = scrolledUp()
        _ = tracker.arrived(arrival())

        XCTAssertTrue(tracker.arrived(arrival(mine: true)))
        XCTAssertEqual(tracker.unreadCount, 0)
        XCTAssertTrue(tracker.isFollowingThread)
    }

    func testTheViewersOwnMessageWhileFollowingNeedsNoExtraScroll() {
        var tracker = ChatScrollTracker()

        XCTAssertFalse(tracker.arrived(arrival(mine: true)))
        XCTAssertEqual(tracker.unreadCount, 0)
        XCTAssertTrue(tracker.isFollowingThread)
    }

    func testReachingTheEndHandsTheThreadBack() {
        var tracker = scrolledUp()
        _ = tracker.arrived(arrival())

        tracker.positionChanged(.init(offset: 500, isAtEnd: true))

        XCTAssertTrue(tracker.isFollowingThread)
        XCTAssertEqual(tracker.unreadCount, 0)
    }

    func testASmallDragThatLeavesTheReaderWhereTheyWereKeepsTheThreadFollowing() {
        var tracker = ChatScrollTracker()
        tracker.positionChanged(.init(offset: 500, isAtEnd: false))
        tracker.dragBegan()
        tracker.positionChanged(.init(offset: 510, isAtEnd: false))
        tracker.scrollSettled()

        XCTAssertTrue(tracker.isFollowingThread)
    }

    func testARequestForEarlierMessagesTakesTheThreadOver() {
        var tracker = ChatScrollTracker()

        tracker.historyRequested()

        XCTAssertFalse(tracker.isFollowingThread)
        XCTAssertFalse(tracker.arrived(arrival()))
    }

    func testTakingThePillFollowsTheThreadAgain() {
        var tracker = scrolledUp()
        _ = tracker.arrived(arrival())

        tracker.followRequested()

        XCTAssertTrue(tracker.isFollowingThread)
        XCTAssertEqual(tracker.unreadCount, 0)
    }

    func testNoArrivalsChangesNothing() {
        var tracker = scrolledUp()

        XCTAssertFalse(tracker.arrived([]))
        XCTAssertEqual(tracker.unreadCount, 0)
    }

    func testADragThatEndsAtTheEndKeepsTheThreadFollowing() {
        var tracker = ChatScrollTracker()
        tracker.dragBegan()
        XCTAssertFalse(tracker.isFollowingThread)

        tracker.scrollSettled()

        XCTAssertTrue(tracker.isFollowingThread)
    }
}
