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

    private func subline(createdAt: Date, role: String? = "creator", canVote: Bool = true, members: Int32 = 8)
        -> String
    {
        CabalHeaderCopy.subline(
            memberCount: members, createdAt: createdAt, role: role, canVote: canVote, now: date(2026, 10, 28),
            calendar: calendar)
    }

    func testTheSublineNamesTheMonthAndYearTheCabalStarted() {
        XCTAssertEqual(
            subline(createdAt: date(2026, 8, 14)), "8 members \u{00B7} since Aug 2026 \u{00B7} you are admin")
    }

    func testTheSublineSaysStartedTodayOnTheCreationDay() {
        XCTAssertEqual(
            subline(createdAt: date(2026, 10, 28, hour: 1), members: 1),
            "1 member \u{00B7} started today \u{00B7} you are admin")
    }

    func testEachRoleReadsItsOwnSuffix() {
        let created = date(2026, 8, 14)
        XCTAssertTrue(subline(createdAt: created, role: "creator").hasSuffix("you are admin"))
        XCTAssertTrue(subline(createdAt: created, role: "admin").hasSuffix("you are admin"))
        XCTAssertTrue(subline(createdAt: created, role: "member", canVote: true).hasSuffix("you vote"))
        XCTAssertTrue(subline(createdAt: created, role: "member", canVote: false).hasSuffix("you hold a share"))
        XCTAssertEqual(subline(createdAt: created, role: nil), "8 members \u{00B7} since Aug 2026")
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
