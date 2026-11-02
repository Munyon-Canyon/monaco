import XCTest

@testable import MonacoCore

final class HomeFigureCopyTests: XCTestCase {
    func testTheNetWorthSplitsIntoWholeDollarsAndDimmedCents() {
        let figure = HomeFigureCopy.netWorth(micros: 12_082_000_000)
        XCTAssertEqual(figure.whole, "$12,082")
        XCTAssertEqual(figure.cents, ".00")
        XCTAssertEqual(HomeFigureCopy.netWorth(micros: 500_123_456).cents, ".12")
    }

    func testEveryRangeNamesItsWindow() {
        let words = LeaderboardRange.figurePeriods.map {
            HomeFigureCopy.line(pnlMicros: 1, returnBps: 1, range: $0).words
        }
        XCTAssertEqual(
            words,
            ["past hour", "past day", "past week", "past month", "past 3 months", "past 6 months", "all time"])
    }

    func testAGainReadsPlusAmountAndUnsignedPercent() {
        let line = HomeFigureCopy.line(pnlMicros: 241_100_000, returnBps: 203, range: .oneMonth)
        XCTAssertEqual(line.amount, "+$241.10")
        XCTAssertEqual(line.percent, "2.0%")
        XCTAssertEqual(line.direction, .up)
    }

    func testALossReadsMinusAmountAndUnsignedPercent() {
        let line = HomeFigureCopy.line(pnlMicros: -241_100_000, returnBps: -203, range: .all)
        XCTAssertEqual(line.amount, "\u{2212}$241.10")
        XCTAssertEqual(line.percent, "2.0%")
        XCTAssertEqual(line.direction, .down)
    }

    func testZeroIsFlat() {
        let line = HomeFigureCopy.line(pnlMicros: 0, returnBps: 0, range: .oneDay)
        XCTAssertEqual(line.amount, "$0.00")
        XCTAssertEqual(line.percent, "0.0%")
        XCTAssertEqual(line.direction, .flat)
    }

    func testSpokenGain() {
        XCTAssertEqual(
            HomeFigureCopy.spoken(
                netWorthMicros: 12_082_000_000, pnlMicros: 241_100_000, returnBps: 203, range: .oneMonth),
            "Net worth, 12,082 dollars, up 241 dollars 10 cents, 2.0 percent, past month")
    }

    func testSpokenLossWithOneDollarAndWholeCents() {
        XCTAssertEqual(
            HomeFigureCopy.spoken(netWorthMicros: 5_000_000, pnlMicros: -1_000_000, returnBps: -500, range: .oneWeek),
            "Net worth, 5 dollars, down 1 dollar, 5.0 percent, past week")
        XCTAssertEqual(
            HomeFigureCopy.spoken(netWorthMicros: 5_000_000, pnlMicros: -10_000, returnBps: -5, range: .oneWeek),
            "Net worth, 5 dollars, down 1 cent, 0.1 percent, past week")
    }

    func testAPnlUnderHalfACentIsFlat() {
        for pnl: Int64 in [-4_999, -1, 1, 4_999] {
            let line = HomeFigureCopy.line(pnlMicros: pnl, returnBps: 0, range: .oneWeek)
            XCTAssertEqual(line.direction, .flat, "\(pnl)")
            XCTAssertEqual(line.amount, "$0.00", "\(pnl)")
            XCTAssertEqual(
                HomeFigureCopy.spoken(netWorthMicros: 5_000_000, pnlMicros: pnl, returnBps: 0, range: .oneWeek),
                "Net worth, 5 dollars, unchanged, past week")
        }
        XCTAssertEqual(HomeFigureCopy.line(pnlMicros: 5_000, returnBps: 1, range: .oneWeek).direction, .up)
        XCTAssertEqual(HomeFigureCopy.line(pnlMicros: -5_000, returnBps: -1, range: .oneWeek).direction, .down)
        XCTAssertEqual(
            HomeFigureCopy.spoken(netWorthMicros: 5_000_000, pnlMicros: -5_000, returnBps: -1, range: .oneWeek),
            "Net worth, 5 dollars, down 1 cent, 0.0 percent, past week")
    }

    func testSpokenFlat() {
        XCTAssertEqual(
            HomeFigureCopy.spoken(netWorthMicros: 5_000_000, pnlMicros: 0, returnBps: 0, range: .sixMonths),
            "Net worth, 5 dollars, unchanged, past 6 months")
    }

    func testTheSixMonthWindowReadsFromItsOwnHistoryRange() {
        XCTAssertEqual(LeaderboardRange.sixMonths.valueHistory, ._6m)
        XCTAssertEqual(LeaderboardRange.threeMonths.valueHistory, ._3m)
        XCTAssertEqual(LeaderboardRange.sixMonths.label, "6M")
        XCTAssertEqual(LeaderboardRange.threeMonths.shortHistoryLine, "Not enough history for the last 3 months yet")
        XCTAssertEqual(LeaderboardRange.sixMonths.shortHistoryLine, "Not enough history for the last 6 months yet")
        XCTAssertEqual(LeaderboardRange.threeMonths.windowPhrase, "the past 3 months")
        XCTAssertEqual(LeaderboardRange.sixMonths.windowPhrase, "the past 6 months")
        XCTAssertEqual(LeaderboardRange.sixMonths.scrubLabelRange, .sixMonths)
        XCTAssertEqual(LeaderboardRange.threeMonths.scrubLabelRange, .threeMonths)
        XCTAssertEqual(LeaderboardRange.threeMonths.generated, .all)
    }
}
