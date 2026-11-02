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
        tracker.positionChanged(.init(distanceFromEnd: 0))
        tracker.dragBegan()
        tracker.positionChanged(.init(distanceFromEnd: 400))
        tracker.dragEnded()
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

        tracker.positionChanged(.init(distanceFromEnd: 0))

        XCTAssertTrue(tracker.isFollowingThread)
        XCTAssertEqual(tracker.unreadCount, 0)
    }

    func testASmallDragThatLeavesTheReaderWhereTheyWereKeepsTheThreadFollowing() {
        var tracker = ChatScrollTracker()
        tracker.positionChanged(.init(distanceFromEnd: 100))
        tracker.dragBegan()
        tracker.positionChanged(.init(distanceFromEnd: 110))
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
        tracker.positionChanged(.init(distanceFromEnd: 300))
        XCTAssertFalse(tracker.isFollowingThread)

        tracker.positionChanged(.init(distanceFromEnd: 0))
        tracker.scrollSettled()

        XCTAssertTrue(tracker.isFollowingThread)
    }

    func testASmallDragKeepsFollowingWhileTheFingerIsStillDown() {
        var tracker = ChatScrollTracker()
        tracker.positionChanged(.init(distanceFromEnd: 0))
        tracker.dragBegan()
        tracker.positionChanged(.init(distanceFromEnd: 20))

        XCTAssertTrue(tracker.isFollowingThread)
    }

    func testAMessageDuringASmallDragIsFollowedNotParkedBehindThePill() {
        var tracker = ChatScrollTracker()
        tracker.positionChanged(.init(distanceFromEnd: 113))
        tracker.dragBegan()
        tracker.positionChanged(.init(distanceFromEnd: 123))

        XCTAssertTrue(tracker.arrived(arrival()))
        XCTAssertEqual(tracker.unreadCount, 0)

        tracker.scrollSettled()

        XCTAssertTrue(tracker.isFollowingThread)
        XCTAssertEqual(tracker.unreadCount, 0)
    }

    func testCoastingJustPastASmallDragStillFollowsAndScrollsToTheNewest() {
        var tracker = ChatScrollTracker()
        tracker.positionChanged(.init(distanceFromEnd: 116))
        tracker.dragBegan()
        tracker.positionChanged(.init(distanceFromEnd: 130))
        tracker.dragEnded()
        tracker.positionChanged(.init(distanceFromEnd: 150))

        XCTAssertTrue(tracker.isFollowingThread)
        XCTAssertTrue(tracker.arrived(arrival()))
        XCTAssertTrue(tracker.scrollSettled())
        XCTAssertTrue(tracker.isFollowingThread)
        XCTAssertEqual(tracker.unreadCount, 0)
    }

    func testASmallDragAtTheEndThatSettlesNearTheEndKeepsFollowing() {
        var tracker = ChatScrollTracker()
        tracker.positionChanged(.init(distanceFromEnd: 0))
        tracker.dragBegan()
        tracker.positionChanged(.init(distanceFromEnd: 30))
        tracker.dragEnded()
        tracker.positionChanged(.init(distanceFromEnd: 12))

        XCTAssertFalse(tracker.scrollSettled())
        XCTAssertTrue(tracker.isFollowingThread)
        XCTAssertEqual(tracker.unreadCount, 0)
    }

    func testAFlickFromTheEndThatLiftsInsideTheSlackButCoastsFarUpLeavesTheReaderInControl() {
        var tracker = ChatScrollTracker()
        tracker.positionChanged(.init(distanceFromEnd: 0))
        tracker.dragBegan()
        tracker.positionChanged(.init(distanceFromEnd: 25))
        tracker.dragEnded()
        tracker.positionChanged(.init(distanceFromEnd: 200))
        tracker.positionChanged(.init(distanceFromEnd: 400))

        XCTAssertFalse(tracker.scrollSettled())
        XCTAssertTrue(tracker.readerControlsScroll)
        XCTAssertFalse(tracker.isFollowingThread)
        XCTAssertFalse(tracker.arrived(arrival()))
        XCTAssertEqual(tracker.unreadCount, 1)
    }

    func testRowsThatArrivedWhileAFlickCoastedShowOnThePillWhenItSettlesFarUp() {
        var tracker = ChatScrollTracker()
        tracker.positionChanged(.init(distanceFromEnd: 0))
        tracker.dragBegan()
        tracker.positionChanged(.init(distanceFromEnd: 25))
        tracker.dragEnded()
        tracker.positionChanged(.init(distanceFromEnd: 150))
        XCTAssertTrue(tracker.arrived(arrival(count: 2)))
        tracker.positionChanged(.init(distanceFromEnd: 400))

        XCTAssertFalse(tracker.scrollSettled())
        XCTAssertTrue(tracker.readerControlsScroll)
        XCTAssertEqual(tracker.unreadCount, 2)
    }

    func testADragFromTheMiddleThatMovesPastTheSlackTakesTheThreadOverEvenAfterTheFingerLifts() {
        var tracker = ChatScrollTracker()
        tracker.positionChanged(.init(distanceFromEnd: 300))
        tracker.dragBegan()
        tracker.positionChanged(.init(distanceFromEnd: 380))
        tracker.dragEnded()
        tracker.positionChanged(.init(distanceFromEnd: 700))

        XCTAssertFalse(tracker.isFollowingThread)
        XCTAssertFalse(tracker.scrollSettled())
        XCTAssertTrue(tracker.readerControlsScroll)
    }

    func testTheOffsetJumpingWhileTheLazyContentIsReestimatedIsNotTheReaderLeaving() {
        var tracker = ChatScrollTracker()
        tracker.positionChanged(.init(distanceFromEnd: 113))
        tracker.dragBegan()
        tracker.positionChanged(.init(distanceFromEnd: 116))

        XCTAssertTrue(tracker.isFollowingThread)
    }

    func testSettlingInsideTheSlackAwayFromTheEndAsksForAScrollToTheNewest() {
        var tracker = ChatScrollTracker()
        tracker.positionChanged(.init(distanceFromEnd: 100))
        tracker.dragBegan()
        tracker.positionChanged(.init(distanceFromEnd: 115))

        XCTAssertTrue(tracker.scrollSettled())
        XCTAssertFalse(tracker.scrollSettled())
    }

    func testADragPastTheSlackTakesTheThreadOver() {
        var tracker = ChatScrollTracker()
        tracker.positionChanged(.init(distanceFromEnd: 0))
        tracker.dragBegan()
        tracker.positionChanged(.init(distanceFromEnd: 200))
        tracker.dragEnded()

        XCTAssertFalse(tracker.isFollowingThread)
        XCTAssertFalse(tracker.arrived(arrival()))
        XCTAssertEqual(tracker.unreadCount, 1)
        XCTAssertFalse(tracker.scrollSettled())
        XCTAssertTrue(tracker.readerControlsScroll)
    }

    func testGrabbingTheThreadAgainWhileItCoastsStartsANewDrag() {
        var tracker = ChatScrollTracker()
        tracker.positionChanged(.init(distanceFromEnd: 0))
        tracker.dragBegan()
        tracker.positionChanged(.init(distanceFromEnd: 10))
        tracker.dragEnded()
        tracker.dragBegan()
        tracker.positionChanged(.init(distanceFromEnd: 300))

        XCTAssertFalse(tracker.isFollowingThread)
    }
}
