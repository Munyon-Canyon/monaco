import Foundation
import MonacoAPI
import MonacoRanking
import XCTest

final class CallBoardCopyTests: XCTestCase {
    private typealias Gap = Components.Schemas.CallBoard.GapPayload

    private func screen(_ board: Components.Schemas.CallBoard = .peopleMonth) -> CallBoardScreen {
        CallBoardScreen(board)
    }

    func testPeriodLinesForMonthWeekAndAllTime() {
        let month = screen()
        XCTAssertEqual(month.periodTitle, "This month")
        XCTAssertEqual(month.periodRange, "Oct 1 – 31")
        var week = Components.Schemas.CallBoard.peopleMonth
        week.period = .init(kind: .week, start: "2026-10-19", end: "2026-10-25")
        XCTAssertEqual(screen(week).periodTitle, "This week")
        XCTAssertEqual(screen(week).periodRange, "Oct 19 – 25")
        let all = screen(.allTimeLeader)
        XCTAssertEqual(all.periodTitle, "All time")
        XCTAssertNil(all.periodRange)
        XCTAssertEqual(CallPeriod.allCases.map(CallBoardCopy.periodLabel), ["Week", "Month", "All time"])
        XCTAssertEqual(CallBoardTab.allCases.map(CallBoardCopy.tabLabel), ["People", "Cabals", "Friends"])
    }

    func testTheCardCentreAndGapForThePeopleMonthFixture() throws {
        let card = try XCTUnwrap(screen().card)
        XCTAssertEqual(card.rankText, "#8")
        XCTAssertEqual(card.ofLine, "of 24 people")
        XCTAssertEqual(card.neighbourLine, "Kai is #7")
        XCTAssertEqual(card.gapLine, "One right call puts you past Kai.")
        XCTAssertEqual(card.me.rate, "78%")
        XCTAssertEqual(card.me.record, "7 of 9 right")
        XCTAssertEqual(card.neighbour?.rate, "79%")
        XCTAssertEqual(card.neighbour?.record, "11 of 14 right")
        XCTAssertEqual(card.climb, 0.7777, accuracy: 0.0001)
    }

    func testTheOfLineNamesFriendsAndCabals() {
        XCTAssertEqual(CallBoardCopy.ofLine(total: 10, tab: .friends), "of 10 friends")
        XCTAssertEqual(CallBoardCopy.ofLine(total: 8, tab: .cabals), "of 8 cabals")
    }

    func testGapCopyCoversEveryCase() {
        func line(_ gap: Gap, _ period: CallPeriod = .month) -> String? {
            CallBoardCopy.gapLine(gap, name: "Kai", period: period)
        }
        XCTAssertEqual(
            line(.init(versus: .above, points: -1, callsToPass: 3, aboveIsPerfect: false)),
            "3 right calls put you past Kai.")
        XCTAssertEqual(
            line(.init(versus: .above, points: -9, callsToPass: nil, aboveIsPerfect: true)),
            "Kai hasn't missed this month.")
        XCTAssertEqual(
            line(.init(versus: .above, points: -9, callsToPass: nil, aboveIsPerfect: true), .week),
            "Kai hasn't missed this week.")
        XCTAssertEqual(
            line(.init(versus: .above, points: -9, callsToPass: nil, aboveIsPerfect: true), .all),
            "Kai hasn't missed yet.")
        XCTAssertNil(line(.init(versus: .above, points: -9, callsToPass: nil, aboveIsPerfect: false)))
        XCTAssertEqual(
            line(.init(versus: .below, points: 1, callsToPass: nil, aboveIsPerfect: false)),
            "You lead. Kai is 1 point behind.")
        XCTAssertEqual(
            line(.init(versus: .below, points: 4, callsToPass: nil, aboveIsPerfect: false)),
            "You lead. Kai is 4 points behind.")
        XCTAssertEqual(
            line(.init(versus: .below, points: 0, callsToPass: nil, aboveIsPerfect: false)),
            "You lead. Kai is level.")
    }

    func testTheLeaderCardNamesTheRowBelow() throws {
        let card = try XCTUnwrap(screen(.allTimeLeader).card)
        XCTAssertEqual(card.rankText, "#1")
        XCTAssertEqual(card.neighbourLine, "Logan is #2")
        XCTAssertEqual(card.gapLine, "You lead. Logan is 1 point behind.")
    }

    func testRowFiguresMovementAndSpeech() throws {
        let board = screen()
        let andres = try XCTUnwrap(board.around.first { $0.name == "Andres" })
        XCTAssertEqual(andres.rate, "78%")
        XCTAssertEqual(andres.record, "7 of 9 right")
        XCTAssertEqual(andres.turnout, "Voted 11 of 11")
        XCTAssertEqual(andres.movementGlyph, "▲ 1")
        XCTAssertEqual(andres.movementSpoken, "Up 1, right on NVDA")
        XCTAssertEqual(andres.spoken, "8, Andres, you, 78 percent, 7 of 9 right, voted 11 of 11, up 1.")
        XCTAssertEqual(andres.movement.signedPlaces, 1)
        XCTAssertTrue(andres.isViewer)
        let rosa = try XCTUnwrap(board.around.first { $0.name == "Rosa" })
        XCTAssertEqual(rosa.movementGlyph, "▼ 1")
        XCTAssertEqual(rosa.movementSpoken, "Down 1, missed TSLA")
        XCTAssertEqual(rosa.movement.signedPlaces, -1)
        XCTAssertTrue(rosa.spoken.hasSuffix("down 1."))
        let mia = try XCTUnwrap(board.top.first { $0.name == "Mia" })
        XCTAssertEqual(mia.movementGlyph, "–")
        XCTAssertNil(mia.movementSpoken)
        XCTAssertNil(mia.movement.signedPlaces)
    }

    func testANewRowAndAnUnrankedRowHaveNoMovementOrRate() {
        XCTAssertEqual(CallBoardCopy.movementGlyph(.new), "–")
        XCTAssertNil(CallBoardCopy.movementSpoken(.new, lastCall: nil))
        XCTAssertEqual(CallBoardCopy.movementSpoken(.up(2), lastCall: nil), "Up 2")
        XCTAssertEqual(CallBoardCopy.rate(nil), "–")
        XCTAssertEqual(
            CallBoardCopy.spoken(
                rank: nil, name: "Zoe", isViewer: false, ratePct: nil, right: 1, resolved: 2, voted: 3, eligible: 4,
                movement: .same),
            "Not ranked, Zoe, 1 of 2 right, voted 3 of 4.")
    }

    func testAroundYouDrawsEveryRowTheResponseHoldsInOrder() {
        XCTAssertEqual(
            screen().around.map(\.name), ["Sam", "Kai", "Andres", "Rosa", "Nora", "Diego", "Maya"])
        XCTAssertEqual(screen().top.map(\.name), ["Christelle", "Logan", "Mia"])
        XCTAssertEqual(CallBoardCopy.topThree, "Top three")
        XCTAssertEqual(CallBoardCopy.aroundYou, "Around you")
    }

    func testTheEmptyBoardAndItsPeriodWord() {
        let empty = screen(.empty)
        XCTAssertTrue(empty.isEmpty)
        XCTAssertNil(empty.card)
        XCTAssertEqual(empty.emptyMessage, "No one has five resolved votes yet this month.")
        XCTAssertEqual(CallBoardCopy.emptyMessage(.week), "No one has five resolved votes yet this week.")
        XCTAssertEqual(CallBoardCopy.emptyMessage(.all), "No one has five resolved votes yet.")
        XCTAssertFalse(screen().isEmpty)
    }

    func testTabsMapToBoardAndFilter() {
        XCTAssertEqual(CallBoardTab.people.board, .people)
        XCTAssertEqual(CallBoardTab.friends.board, .people)
        XCTAssertEqual(CallBoardTab.friends.filter, .friends)
        XCTAssertEqual(CallBoardTab.cabals.board, .cabals)
        XCTAssertEqual(CallBoardTab.cabals.filter, .all)
        XCTAssertEqual(CallBoardTab(board: .people, filter: .friends), .friends)
        XCTAssertEqual(CallBoardTab(board: .cabals, filter: .all), .cabals)
        XCTAssertEqual(CallBoardTab(board: .people, filter: .all), .people)
    }

    func testNoScreenStringMentionsSeasonMoneyOrReturns() {
        let strings =
            [
                CallBoardCopy.title, CallBoardCopy.topThree, CallBoardCopy.aroundYou, CallBoardCopy.errorTitle,
                CallBoardCopy.errorMessage, CallBoardCopy.emptyTitle, screen().emptyMessage,
                screen().periodTitle, screen().card?.gapLine ?? "",
            ] + screen().around.map(\.spoken)
        for text in strings {
            for banned in ["Season", "P&L", "$", "return"] {
                XCTAssertFalse(text.contains(banned), "\(text) contains \(banned)")
            }
        }
    }
}
