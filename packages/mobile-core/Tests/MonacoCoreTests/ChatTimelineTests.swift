import Foundation
import XCTest

@testable import MonacoCore

final class ChatTimelineTests: XCTestCase {
    private typealias Fixtures = ChatFixtures

    private var calendar: Calendar { .current }

    func testRowsRunOldestFirstWithTiesBrokenByID() {
        var timeline = ChatTimeline(viewerID: Fixtures.viewerID)

        timeline.mergeNewest(
            [Fixtures.message("b", minutes: 1), Fixtures.message("a", minutes: 1), Fixtures.message("c", minutes: 0)],
            pageSize: 50
        )

        XCTAssertEqual(timeline.rows.map(\.id), ["c", "a", "b"])
    }

    func testARunByOneAuthorStartsAndEndsOnItsFirstAndLastRow() {
        var timeline = ChatTimeline(viewerID: Fixtures.viewerID)

        timeline.mergeNewest(
            [
                Fixtures.message("m1", minutes: 1),
                Fixtures.message("m2", minutes: 2),
                Fixtures.message("m3", author: Fixtures.viewerID, minutes: 3),
            ],
            pageSize: 50
        )

        XCTAssertEqual(timeline.rows.map(\.startsRun), [true, false, true])
        XCTAssertEqual(timeline.rows.map(\.endsRun), [false, true, true])
        XCTAssertEqual(timeline.rows.map(\.isMine), [false, false, true])
    }

    func testADayDividerSitsBetweenMessagesOnDifferentDays() throws {
        var timeline = ChatTimeline(viewerID: Fixtures.viewerID)
        let noon = calendar.startOfDay(for: Fixtures.epoch).addingTimeInterval(12 * 3_600)
        let yesterday = Fixtures.message("m1")
        var today = Fixtures.message("m2")
        var earlier = yesterday
        earlier.createdAt = noon.addingTimeInterval(-86_400)
        today.createdAt = noon

        timeline.mergeNewest([earlier, today], pageSize: 50)

        XCTAssertEqual(timeline.rows.map(\.startsDay), [true, true])
        XCTAssertEqual(timeline.rows.map(\.startsRun), [true, true])
        XCTAssertEqual(timeline.rows.map(\.endsRun), [true, true])
        XCTAssertEqual(timeline.rows[0].dayLabel(now: noon), "Yesterday")
        XCTAssertEqual(timeline.rows[1].dayLabel(now: noon), "Today")
    }

    func testTwoMessagesOnOneDayShareADivider() {
        var timeline = ChatTimeline(viewerID: Fixtures.viewerID)
        let morning = calendar.startOfDay(for: Fixtures.epoch).addingTimeInterval(3_600)
        var first = Fixtures.message("m1")
        var second = Fixtures.message("m2")
        first.createdAt = morning
        second.createdAt = morning.addingTimeInterval(600)

        timeline.mergeNewest([first, second], pageSize: 50)

        XCTAssertEqual(timeline.rows.map(\.startsDay), [true, false])
        XCTAssertNil(timeline.rows[1].dayLabel(now: morning))
    }

    func testDayLabelNamesOlderDaysByDate() {
        let now = calendar.startOfDay(for: Fixtures.epoch).addingTimeInterval(12 * 3_600)
        let weekAgo = now.addingTimeInterval(-7 * 86_400)
        let yearAgo = now.addingTimeInterval(-400 * 86_400)
        let locale = Locale(identifier: "en_US")

        XCTAssertFalse(ChatDayLabel.text(for: weekAgo, now: now, calendar: calendar, locale: locale).isEmpty)
        XCTAssertNotEqual(
            ChatDayLabel.text(for: weekAgo, now: now, calendar: calendar, locale: locale),
            ChatDayLabel.text(for: yearAgo, now: now, calendar: calendar, locale: locale)
        )
        XCTAssertTrue(
            ChatDayLabel.text(for: yearAgo, now: now, calendar: calendar, locale: locale).contains(
                String(calendar.component(.year, from: yearAgo))))
    }

    func testMergingAKnownMessageAddsNothingAndLeavesRowsAlone() {
        var timeline = ChatTimeline(viewerID: Fixtures.viewerID)
        timeline.mergeNewest([Fixtures.message("m1")], pageSize: 50)
        let before = timeline.rows

        let added = timeline.mergeNewer([Fixtures.message("m1")])

        XCTAssertTrue(added.isEmpty)
        XCTAssertEqual(timeline.rows, before)
    }

    func testAFetchedPageRefreshesAMessageItAlreadyHolds() {
        var timeline = ChatTimeline(viewerID: Fixtures.viewerID)
        timeline.mergeNewest([Fixtures.message("m1")], pageSize: 50)

        timeline.mergeNewer([Fixtures.message("m1", replyCount: 3)])

        XCTAssertEqual(timeline.rows.first?.message.replyCount, 3)
    }

    func testALiveMessageAlreadyHeldIsDroppedWithoutOverwritingIt() {
        var timeline = ChatTimeline(viewerID: Fixtures.viewerID)
        timeline.mergeNewest([Fixtures.message("m1", replyCount: 3)], pageSize: 50)

        let inserted = timeline.insertLive(Fixtures.message("m1", replyCount: 0))

        XCTAssertFalse(inserted)
        XCTAssertEqual(timeline.rows.first?.message.replyCount, 3)
    }

    func testAFullFirstPageSaysThereIsHistoryAndAShortOneSaysThereIsNot() {
        var full = ChatTimeline(viewerID: Fixtures.viewerID)
        var short = ChatTimeline(viewerID: Fixtures.viewerID)

        full.mergeNewest((0..<3).map { Fixtures.message("m\($0)", minutes: Double($0)) }, pageSize: 3)
        short.mergeNewest((0..<2).map { Fixtures.message("m\($0)", minutes: Double($0)) }, pageSize: 3)

        XCTAssertTrue(full.hasOlder)
        XCTAssertFalse(short.hasOlder)
    }

    func testAnEmptyPageStillCountsAsLoaded() {
        var timeline = ChatTimeline(viewerID: Fixtures.viewerID)
        XCTAssertFalse(timeline.hasLoadedNewest)

        timeline.mergeNewest([], pageSize: 50)

        XCTAssertTrue(timeline.hasLoadedNewest)
        XCTAssertNil(timeline.newestID)
    }

    private func rows(_ messages: [ChatMessage]) -> [ChatRow] {
        var timeline = ChatTimeline(viewerID: Fixtures.viewerID)
        timeline.mergeNewest(messages, pageSize: 50)
        return timeline.rows
    }

    func testAMessageAfterTheNewestOneArrivesAndTheFirstLoadDoesNot() {
        let before = rows([Fixtures.message("m1", minutes: 1)])
        let after = rows([Fixtures.message("m1", minutes: 1), Fixtures.message("m2", minutes: 2)])

        XCTAssertEqual(ChatArrivals.added(from: before, to: after).map(\.id), ["m2"])
        XCTAssertTrue(ChatArrivals.added(from: [], to: after).isEmpty)
    }

    func testAPageOfOlderHistoryIsNotAnArrival() {
        let before = rows([Fixtures.message("m5", minutes: 5)])
        let after = rows([Fixtures.message("m1", minutes: 1), Fixtures.message("m5", minutes: 5)])

        XCTAssertTrue(ChatArrivals.added(from: before, to: after).isEmpty)
    }

    func testOwnMessagesAlwaysFollowAndOthersOnlyWhileFollowing() {
        let others = rows([Fixtures.message("m1")])
        let own = rows([Fixtures.message("m2", author: Fixtures.viewerID)])

        XCTAssertTrue(ChatArrivals.shouldFollow(added: own, isFollowingThread: false))
        XCTAssertFalse(ChatArrivals.shouldFollow(added: others, isFollowingThread: false))
        XCTAssertTrue(ChatArrivals.shouldFollow(added: others, isFollowingThread: true))
        XCTAssertFalse(ChatArrivals.shouldFollow(added: [], isFollowingThread: true))
    }

    func testAnUnsentRowShowsPendingThenFailedAndAnEchoSettlesIt() {
        var timeline = ChatTimeline(viewerID: Fixtures.viewerID)
        timeline.mergeNewest([Fixtures.message("m1", minutes: 1)], pageSize: 50)
        timeline.addUnsent(key: "k1", body: "gm", at: Fixtures.epoch.addingTimeInterval(3_600))
        XCTAssertEqual(timeline.rows.map(\.delivery), [.sent, .pending])

        timeline.setFailed(key: "k1", true)
        XCTAssertEqual(timeline.rows.last?.delivery, .failed)
        XCTAssertEqual(timeline.unsent(key: "k1")?.body, "gm")

        timeline.insertLive(Fixtures.message("m2", author: Fixtures.viewerID, body: "gm", minutes: 61))
        XCTAssertEqual(timeline.rows.map(\.id), ["m1", "m2", "k1"])

        timeline.setFailed(key: "k1", false)
        timeline.insertLive(Fixtures.message("m3", author: Fixtures.viewerID, body: "gm", minutes: 62))
        XCTAssertEqual(timeline.rows.map(\.id), ["m1", "m2", "m3"])
        XCTAssertNil(timeline.unsent(key: "k1"))
    }

    func testSettlingASendReplacesItsRowAndDroppingRemovesIt() {
        var timeline = ChatTimeline(viewerID: Fixtures.viewerID)
        timeline.addUnsent(key: "k1", body: "gm", at: Fixtures.epoch)
        timeline.settle(Fixtures.message("m1", author: Fixtures.viewerID, body: "gm"), key: "k1")
        XCTAssertEqual(timeline.rows.map(\.id), ["m1"])

        timeline.addUnsent(key: "k2", body: "x", at: Fixtures.epoch)
        timeline.dropUnsent(key: "k2")
        XCTAssertEqual(timeline.rows.map(\.id), ["m1"])
    }

    func testDeletingAMessageWithRepliesLeavesAPlaceholderAndOneWithoutRemovesIt() {
        var timeline = ChatTimeline(viewerID: Fixtures.viewerID)
        timeline.mergeNewest(
            [Fixtures.message("m1", minutes: 1, replyCount: 2), Fixtures.message("m2", minutes: 2)], pageSize: 50)

        timeline.markDeleted(id: "m1")
        timeline.markDeleted(id: "m2")

        XCTAssertEqual(timeline.rows.map(\.id), ["m1"])
        XCTAssertTrue(timeline.rows[0].isPlaceholder)
        XCTAssertNil(timeline.rows[0].message.body)
    }

    func testAThreadUpdateChangesTheReplyCount() {
        var timeline = ChatTimeline(viewerID: Fixtures.viewerID)
        timeline.mergeNewest([Fixtures.message("m1")], pageSize: 50)

        timeline.applyThread(id: "m1", replyCount: 4, lastReplyAt: Fixtures.epoch)

        XCTAssertEqual(timeline.rows[0].message.replyCount, 4)
    }
}
