import MonacoAPI
import XCTest

@testable import MonacoCore

final class ProfileStatsTests: XCTestCase {
    func testTwoCabalsShowTheTotalTheGainWithItsReturnAndTheCount() {
        let stats = ProfileStats(.sample)

        XCTAssertEqual(stats.inCabals, "$1,000.00")
        XCTAssertEqual(stats.allTime, "+$14.00 · +1.42%")
        XCTAssertEqual(stats.allTimePnl, "+$14.00")
        XCTAssertEqual(stats.cabals, "2")
    }

    func testNoCabalsIsZeroDollarsTwiceAndZero() {
        let stats = ProfileStats(.sampleEmpty)

        XCTAssertEqual(stats.inCabals, "$0.00")
        XCTAssertEqual(stats.allTime, "$0.00")
        XCTAssertEqual(stats.cabals, "0")
    }

    func testANullReturnShowsTheAmountAlone() {
        var portfolio = Components.Schemas.MyPortfolio.sample
        portfolio.returnBps = nil

        XCTAssertEqual(ProfileStats(portfolio).allTime, "+$14.00")
    }

    func testALossUsesTheTypographicMinusOnBothFigures() {
        let stats = ProfileStats(.sampleLoss)

        XCTAssertEqual(stats.inCabals, "$949.00")
        XCTAssertEqual(stats.allTime, "\u{2212}$1.00 · \u{2212}0.11%")
        XCTAssertEqual(stats.allTimePnl, "\u{2212}$1.00")
        XCTAssertFalse(stats.allTime.contains("-"))
    }

    func testCabalsCountsEveryEntryEvenOneWithNoValue() {
        XCTAssertEqual(ProfileStats(.sampleLoss).cabals, "2")
    }
}
