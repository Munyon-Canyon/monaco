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
        XCTAssertEqual(CallBoardCopy.emptyMessage(.week, tab: .people), "No one has five resolved votes yet this week.")
        XCTAssertEqual(CallBoardCopy.emptyMessage(.all, tab: .people), "No one has five resolved votes yet.")
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

    func testCabalGapLinesCoverBehindLevelAndLeads() {
        func line(_ versus: Gap.VersusPayload, _ points: Int32) -> String {
            CallBoardCopy.cabalGapLine(
                .init(versus: versus, points: points, callsToPass: nil, aboveIsPerfect: false),
                me: "Tech Titans", other: "Pilates")
        }
        XCTAssertEqual(line(.above, -4), "Tech Titans is 4 points behind Pilates.")
        XCTAssertEqual(line(.above, -1), "Tech Titans is 1 point behind Pilates.")
        XCTAssertEqual(line(.above, 0), "Tech Titans is level with Pilates.")
        XCTAssertEqual(line(.below, 0), "Tech Titans is level with Pilates.")
        XCTAssertEqual(line(.below, 1), "Tech Titans leads by 1 point.")
        XCTAssertEqual(line(.below, 3), "Tech Titans leads by 3 points.")
    }

    func testPendingLineCoversZeroToFourSymbols() {
        XCTAssertEqual(CallBoardCopy.pendingLine([]), "Vote on your cabals' proposals to get there.")
        XCTAssertEqual(CallBoardCopy.pendingLine(["AMZN"]), "Your AMZN vote counts once it resolves.")
        XCTAssertEqual(
            CallBoardCopy.pendingLine(["SpaceX", "AMZN"]), "Your SpaceX and AMZN votes count once they resolve.")
        XCTAssertEqual(
            CallBoardCopy.pendingLine(["SpaceX", "AMZN", "NVDA"]),
            "Your SpaceX, AMZN and 1 more vote count once they resolve.")
        XCTAssertEqual(
            CallBoardCopy.pendingLine(["SpaceX", "AMZN", "NVDA", "TSLA"]),
            "Your SpaceX, AMZN and 2 more votes count once they resolve.")
    }

    func testTurnoutAndYoursOnCabalRows() {
        XCTAssertEqual(CallBoardCopy.turnoutPercent(bps: 9_200), "Turnout 92%")
        XCTAssertEqual(CallBoardCopy.turnoutPercent(bps: 9_166), "Turnout 92%")
        XCTAssertEqual(CallBoardCopy.name("Tech Titans", isMine: true), "Tech Titans · yours")
        XCTAssertEqual(CallBoardCopy.name("Pilates", isMine: false), "Pilates")
    }

    func testTheCabalsMonthScreenMatchesTheFrame() throws {
        let cabals = screen(.cabalsMonth)
        let card = try XCTUnwrap(cabals.card)
        XCTAssertEqual(card.rankText, "#2")
        XCTAssertEqual(card.ofLine, "of 8 cabals")
        XCTAssertEqual(card.neighbourLine, "your best")
        XCTAssertEqual(card.gapLine, "Tech Titans is 4 points behind Pilates.")
        XCTAssertNil(card.meLabel)
        XCTAssertEqual(cabals.aroundTitle, "Around your cabals")
        XCTAssertEqual(
            cabals.around.map(\.title),
            ["Tech Titans · yours", "AC Capital · yours", "Degens · yours", "Quant Kids", "Night Shift"])
        XCTAssertEqual(cabals.around.map(\.turnout).prefix(3), ["Turnout 92%", "Turnout 94%", "Turnout 88%"])
        XCTAssertTrue(cabals.around.allSatisfy(\.isCabal))
        XCTAssertEqual(cabals.bandID, cabals.around.first?.id)
        XCTAssertNil(cabals.unrankedCard)
        XCTAssertEqual(screen().card?.meLabel, "You")
        XCTAssertEqual(screen().aroundTitle, "Around you")
    }

    func testTheUnrankedPersonCardAndSections() throws {
        let week = screen(.peopleWeekUnranked)
        XCTAssertNil(week.card)
        XCTAssertFalse(week.isEmpty)
        let card = try XCTUnwrap(week.unrankedCard)
        XCTAssertEqual(card.title, "Not ranked yet this week")
        XCTAssertEqual(card.progress, "3 of 5 resolved votes · 2 more to rank")
        XCTAssertEqual(card.pending, "Your SpaceX and AMZN votes count once they resolve.")
        XCTAssertEqual(card.climb, 0.6, accuracy: 0.0001)
        XCTAssertEqual(card.subject.name, "Andres")
        XCTAssertEqual(week.aroundTitle, "Ranked")
        XCTAssertEqual(week.around.map(\.name), ["Kai", "Jordan"])
        XCTAssertEqual(week.unrankedRows.map(\.name), ["Andres", "Rosa"])
        let andres = try XCTUnwrap(week.unrankedRows.first)
        XCTAssertEqual(andres.unrankedSubtitle, "3 resolved · voted 5 of 6")
        XCTAssertEqual(andres.moreFigure, "2 more")
        XCTAssertEqual(andres.unrankedSpoken, "Andres, you, not ranked yet, 3 resolved, 2 more to rank.")
        XCTAssertEqual(week.unrankedRows.last?.unrankedSpoken, "Rosa, not ranked yet, 2 resolved, 3 more to rank.")
        XCTAssertEqual(week.unrankedRows.last?.moreFigure, "3 more")
    }

    func testTheUnrankedCardTitlePerPeriodAndCabalForm() throws {
        XCTAssertEqual(CallBoardCopy.unrankedCardTitle(period: .month), "Not ranked yet this month")
        XCTAssertEqual(CallBoardCopy.unrankedCardTitle(period: .all), "Not ranked yet")
        let card = try XCTUnwrap(screen(.cabalsUnranked).unrankedCard)
        XCTAssertEqual(card.title, "Tech Titans is not ranked yet")
        XCTAssertEqual(card.progress, "3 of 5 resolved")
        XCTAssertEqual(card.pending, "Your SpaceX vote counts once it resolves.")
        XCTAssertTrue(card.subject.isCabal)
        XCTAssertEqual(CallBoardCopy.unrankedProgress(resolved: 3, needed: 2), "3 of 5 resolved votes · 2 more to rank")
        XCTAssertEqual(screen(.cabalsUnranked).aroundTitle, "Ranked")
    }

    func testFriendsCopyForTheFriendsMonthFixture() throws {
        let friends = screen(.friendsMonth)
        let card = try XCTUnwrap(friends.card)
        XCTAssertEqual(friends.tab, .friends)
        XCTAssertEqual(card.rankText, "#6")
        XCTAssertEqual(card.ofLine, "of 10 friends")
        XCTAssertEqual(card.neighbourLine, "Kai is #5")
        XCTAssertEqual(card.gapLine, "One right call puts you past Kai.")
        XCTAssertEqual(friends.podiumTitle, "Top of your friends")
        XCTAssertEqual(screen().podiumTitle, "Top three")
        XCTAssertEqual(friends.around.compactMap(\.rank), Array(4...10))
        XCTAssertEqual(friends.around.first?.name, "Jordan")
        XCTAssertEqual(friends.around.first?.record, "7 of 8 right")
        XCTAssertNil(friends.afterCursor)
    }

    func testFriendsEmptyStateNamesFriendsForEveryPeriod() {
        var board = Components.Schemas.CallBoard.empty
        board.filter = .friends
        XCTAssertEqual(
            screen(board).emptyMessage, "None of your friends has five resolved votes yet this month.")
        XCTAssertEqual(
            CallBoardCopy.emptyMessage(.week, tab: .friends),
            "None of your friends has five resolved votes yet this week.")
        XCTAssertEqual(
            CallBoardCopy.emptyMessage(.all, tab: .friends), "None of your friends has five resolved votes yet.")
    }

    func testTheInfoSheetHoldsTheFiveLines() {
        XCTAssertEqual(CallBoardCopy.infoTitle, "How the board works")
        XCTAssertEqual(CallBoardCopy.infoLines.count, 5)
        XCTAssertEqual(CallBoardCopy.infoLines[2], "Five resolved votes to rank.")
    }
}
