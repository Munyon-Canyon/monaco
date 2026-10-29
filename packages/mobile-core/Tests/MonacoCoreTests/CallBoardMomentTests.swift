import Foundation
import MonacoAPI
import MonacoRanking
import XCTest

final class CallBoardMomentTests: XCTestCase {
    private final class Backing: CallBoardSeenBacking {
        var values: [String: String] = [:]
        var failsReads = false
        func read(_ key: String) throws -> String? {
            if failsReads { throw URLError(.badURL) }
            return values[key]
        }
        func write(_ key: String, _ value: String) { values[key] = value }
    }

    private func moment(_ board: Components.Schemas.CallBoard = .peopleMonthLeadChange) throws -> CallBoardMoment {
        try XCTUnwrap(CallBoardScreen(board).moment)
    }

    func testCrownCopyWithTheCall() throws {
        let moment = try moment()
        XCTAssertEqual(moment.title, "Christelle took the lead")
        XCTAssertEqual(moment.subline, "Past Logan, right on Buy META · 92%")
        XCTAssertEqual(moment.glyph, "▲1")
        XCTAssertEqual(moment.spoken, "Christelle took the lead from Logan, right on Buy META, 92 percent.")
    }

    func testCrownCopyWithoutTheCallNeverClaimsRightOn() throws {
        let moment = try moment(.peopleMonthLeadChangeNoCall)
        XCTAssertEqual(moment.subline, "Past Logan · 92%")
        XCTAssertFalse(moment.subline.contains("right on"))
        XCTAssertEqual(moment.spoken, "Christelle took the lead from Logan, 92 percent.")
    }

    func testCabalsVariantAndNoMovementGlyph() {
        XCTAssertEqual(CallBoardCopy.leadTitle(leader: "Night Owls", isCabal: true), "Night Owls took the lead")
        XCTAssertEqual(
            CallBoardCopy.leadSubline(
                passed: "Day Traders", isCabal: true, call: (kind: "Sell", symbol: "TSLA"), ratePct: 80),
            "Past Day Traders, right on Sell TSLA · 80%")
        XCTAssertEqual(
            CallBoardCopy.leadSpoken(
                leader: "Night Owls", passed: "Day Traders", isCabal: true, call: nil, ratePct: 80),
            "Night Owls took the lead from Day Traders, 80 percent.")
        XCTAssertNil(CallBoardCopy.leadGlyph(places: 0))
    }

    func testSeenOnce() throws {
        let backing = Backing()
        let store = CallBoardSeenStore(backing: backing)
        let first = try moment()
        XCTAssertTrue(store.shouldShow(first))
        store.markSeen(first)
        XCTAssertFalse(store.shouldShow(first))
        XCTAssertTrue(store.shouldShow(try moment(.peopleMonthLeadChangeNoCall)))
    }

    func testAFailedReadShowsTheCardAgain() throws {
        let backing = Backing()
        let store = CallBoardSeenStore(backing: backing)
        let first = try moment()
        store.markSeen(first)
        backing.failsReads = true
        XCTAssertTrue(store.shouldShow(first))
    }

    func testUserDefaultsBacking() throws {
        let defaults = try XCTUnwrap(UserDefaults(suiteName: "CallBoardMomentTests"))
        defaults.removePersistentDomain(forName: "CallBoardMomentTests")
        let store = CallBoardSeenStore(backing: defaults)
        let first = try moment()
        XCTAssertTrue(store.shouldShow(first))
        store.markSeen(first)
        XCTAssertFalse(store.shouldShow(first))
        defaults.removePersistentDomain(forName: "CallBoardMomentTests")
    }

    func testNoLeadChangeMeansNoMoment() {
        XCTAssertNil(CallBoardScreen(.peopleMonth).moment)
    }

    func testStandingShareText() throws {
        let card = try XCTUnwrap(CallBoardScreen(.peopleMonthLeadChange).card)
        XCTAssertEqual(card.standing, "I'm #8 of 24 on Monaco this month · 78% right calls")
        XCTAssertEqual(CallBoardCopy.shareStanding, "Share your standing")
        XCTAssertEqual(
            CallBoardCopy.standing(rank: 1, total: 3, ratePct: nil, period: .all, tab: .people),
            "I'm #1 of 3 on Monaco all time")
    }

    func testUnrankedViewerHasNoStanding() throws {
        var board = Components.Schemas.CallBoard.peopleMonthLeadChange
        board.me?.rank = nil
        XCTAssertNil(try XCTUnwrap(CallBoardScreen(board).card).standing)
    }

    func testDockedShowsOnlyWhenTheViewerRowIsOffScreen() {
        let listed = CallBoardScreen(.peopleMonth)
        XCTAssertFalse(listed.showsDocked(viewerRowVisible: true))
        XCTAssertTrue(listed.showsDocked(viewerRowVisible: false))
        XCTAssertTrue(CallBoardScreen(.peopleMonthLeadChange).showsDocked(viewerRowVisible: true))
        XCTAssertFalse(CallBoardScreen(.empty).showsDocked(viewerRowVisible: false))
    }
}
