import Foundation
import MonacoCore
import XCTest

final class CabalHeaderCopyTests: XCTestCase {
    private var calendar: Calendar {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = TimeZone(identifier: "UTC") ?? .gmt
        return calendar
    }

    private func date(_ year: Int, _ month: Int, _ day: Int, hour: Int = 12) -> Date {
        calendar.date(from: DateComponents(year: year, month: month, day: day, hour: hour)) ?? .distantPast
    }

    private func line(_ names: [String], members: Int32, createdAt: Date) -> String {
        CabalHeaderCopy.peopleLine(
            names: names, memberCount: members, createdAt: createdAt, now: date(2026, 10, 28), calendar: calendar)
    }

    func testTheLineNamesTwoPeopleAndCountsTheOthers() {
        XCTAssertEqual(
            line(["Christelle", "Logan"], members: 8, createdAt: date(2026, 8, 14)),
            "Christelle, Logan and 6 others \u{00B7} since August")
        XCTAssertEqual(
            line(["Christelle", "Logan"], members: 3, createdAt: date(2026, 8, 14)),
            "Christelle, Logan and 1 other \u{00B7} since August")
        XCTAssertEqual(
            line(["Christelle", "Logan"], members: 4, createdAt: date(2026, 8, 14)),
            "Christelle, Logan and 2 others \u{00B7} since August")
    }

    func testTheLineReadsShorterWithFewerPeople() {
        XCTAssertEqual(
            line(["Christelle"], members: 2, createdAt: date(2026, 8, 14)), "Christelle \u{00B7} since August")
        XCTAssertEqual(
            line(["Christelle", "Logan"], members: 3, createdAt: date(2026, 8, 14)).prefix(10), "Christelle")
        XCTAssertEqual(
            CabalHeaderCopy.peopleNames(names: ["Christelle", "Logan"], memberCount: 2), "Christelle and Logan")
    }

    func testTheLineFallsBackToTheCountWithoutOthers() {
        XCTAssertEqual(line([], members: 1, createdAt: date(2026, 8, 14)), "1 member \u{00B7} since August")
        XCTAssertEqual(line([], members: 0, createdAt: date(2026, 8, 14)), "1 member \u{00B7} since August")
    }

    func testTheLineSaysStartedTodayOnTheCreationDay() {
        XCTAssertEqual(
            line([], members: 1, createdAt: date(2026, 10, 28, hour: 1)), "1 member \u{00B7} started today")
        XCTAssertEqual(
            line(["Christelle"], members: 2, createdAt: date(2026, 10, 28, hour: 1)),
            "Christelle \u{00B7} started today")
    }

    private func row(_ id: String, role: String) -> CabalMemberRow {
        CabalMemberRow(
            .init(
                userId: id, handle: nil, displayName: id, photoUrl: nil, role: role, canVote: true,
                joinedAt: date(2026, 8, 1)))
    }

    func testTheHeaderMembersDropTheViewerAndPutLeadsFirst() {
        let rows = [
            row("v", role: "creator"), row("m1", role: "member"), row("a1", role: "admin"), row("m2", role: "member"),
        ]
        XCTAssertEqual(CabalHeaderCopy.headerMembers(rows: rows, viewerID: "v").map(\.id), ["a1", "m1", "m2"])
        XCTAssertEqual(
            CabalHeaderCopy.headerMembers(rows: rows, viewerID: "m1").map(\.id), ["v", "a1", "m2"])
        XCTAssertEqual(
            CabalHeaderCopy.headerMembers(rows: rows, viewerID: nil).map(\.id), ["v", "a1", "m1", "m2"])
        XCTAssertEqual(CabalHeaderCopy.headerMembers(rows: [rows[0]], viewerID: "v").count, 0)
    }

    func testEachRoleReadsItsOwnViewerClause() {
        XCTAssertEqual(CabalHeaderCopy.viewer(role: "creator", canVote: true), "you are admin")
        XCTAssertEqual(CabalHeaderCopy.viewer(role: "admin", canVote: true), "you are admin")
        XCTAssertEqual(CabalHeaderCopy.viewer(role: "member", canVote: true), "you vote")
        XCTAssertEqual(CabalHeaderCopy.viewer(role: "member", canVote: false), "you hold a share")
        XCTAssertNil(CabalHeaderCopy.viewer(role: nil, canVote: true))
    }

    func testTheStatBlockReadsShareSliceAndVotes() {
        let stats = CabalHeaderCopy.stats(shareMicros: 7_000_000_000, sliceBps: 3500, openVotes: 1)
        XCTAssertEqual(stats.map(\.label), ["Your share", "Of treasury", "Open votes"])
        XCTAssertEqual(stats.map(\.value), ["$7,000", "35%", "1"])
    }

    func testAMemberWithNoShareSeesZeroDollarsAndZeroPercent() {
        let stats = CabalHeaderCopy.stats(shareMicros: 0, sliceBps: 0, openVotes: 0)
        XCTAssertEqual(stats.map(\.value), ["$0", "0%", "0"])
    }

    func testTheShareDropsCentsAndASmallSliceReadsUnderOnePercent() {
        let stats = CabalHeaderCopy.stats(shareMicros: 1_234_990_000, sliceBps: 40, openVotes: 3)
        XCTAssertEqual(stats[0].value, "$1,234")
        XCTAssertEqual(stats[1].value, "<1%")
    }

    func testTheBotCellStaysHiddenWhileTradingBotsAreNotLaunched() {
        XCTAssertFalse(LaunchFeatures.bots)
        XCTAssertEqual(CabalHeaderCopy.botValue, "Off")
        XCTAssertFalse(CabalHeaderCopy.stats(shareMicros: 1, sliceBps: 1, openVotes: 1).contains { $0.label == "Bot" })
    }

    private func curve(_ navs: [Int64]) -> ValueCurve {
        ValueCurve(
            samples: navs.enumerated().map {
                .init(at: date(2026, 9, 1 + $0.offset), value: 0, pnl: 0, nav: $0.element)
            })
    }

    func testTheMoveIsTheNavChangeOverTheRangeToOneDecimal() throws {
        let move = try XCTUnwrap(
            CabalHeaderCopy.move(
                curve([1_000_000, 959_000]), createdAt: date(2026, 8, 1), now: date(2026, 10, 28), calendar: calendar))
        XCTAssertEqual(move.percent, "\u{2212}4.1%")
        XCTAssertEqual(move.direction, .down)
        XCTAssertEqual(move.line, "\u{2212}4.1% \u{00B7} 1M")
    }

    func testAGainReadsWithAPlus() throws {
        let move = try XCTUnwrap(
            CabalHeaderCopy.move(
                curve([1_000_000, 1_140_000]), createdAt: date(2026, 8, 1), now: date(2026, 10, 28),
                calendar: calendar))
        XCTAssertEqual(move.percent, "+14.0%")
        XCTAssertEqual(move.direction, .up)
    }

    func testAMoveUnderHalfACentIsFlatAndAtHalfACentItHasADirection() throws {
        let under = try XCTUnwrap(
            CabalHeaderCopy.move(
                curve([1_000_000, 995_001]), createdAt: date(2026, 8, 1), now: date(2026, 10, 28), calendar: calendar))
        let at = try XCTUnwrap(
            CabalHeaderCopy.move(
                curve([1_000_000, 995_000]), createdAt: date(2026, 8, 1), now: date(2026, 10, 28), calendar: calendar))
        XCTAssertEqual(under.direction, .flat)
        XCTAssertEqual(at.direction, .down)
    }

    func testThereIsNoMoveInTheFirstMonthOrWithoutHistory() {
        let rising = curve([1_000_000, 1_100_000])
        XCTAssertNil(
            CabalHeaderCopy.move(rising, createdAt: date(2026, 10, 10), now: date(2026, 10, 28), calendar: calendar))
        XCTAssertNil(
            CabalHeaderCopy.move(
                curve([1_000_000]), createdAt: date(2026, 8, 1), now: date(2026, 10, 28), calendar: calendar))
        XCTAssertNil(
            CabalHeaderCopy.move(nil, createdAt: date(2026, 8, 1), now: date(2026, 10, 28), calendar: calendar))
    }

    func testTheShortNameDropsATrailingCompanyWord() {
        XCTAssertEqual(CabalHeaderCopy.shortName("AC Capital Management"), "AC Capital")
        XCTAssertEqual(CabalHeaderCopy.shortName("Semis or bust"), "Semis or bust")
        XCTAssertEqual(CabalHeaderCopy.shortName("Management"), "Management")
        XCTAssertEqual(CabalHeaderCopy.shortName("Kai Partners Group"), "Kai")
    }

    func testCompactDollarsTrimWholeAmounts() {
        XCTAssertEqual(CabalHeaderCopy.compactDollars(400_000_000), "$400")
        XCTAssertEqual(CabalHeaderCopy.compactDollars(400_500_000), "$400.50")
    }
}
