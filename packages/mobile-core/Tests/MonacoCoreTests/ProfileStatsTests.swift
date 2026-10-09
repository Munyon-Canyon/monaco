import MonacoAPI
import XCTest

@testable import MonacoCore

final class ProfileStatsTests: XCTestCase {
    func testTwoCabalsShowTheTotalTheGainWithItsReturnAndTheCount() {
        let stats = ProfileStats(.sample)

        XCTAssertEqual(stats.inCabals, "$1,000.00")
        XCTAssertEqual(stats.returnDollars, "+$14.00")
        XCTAssertEqual(stats.returnPercent, "+1.4%")
        XCTAssertEqual(stats.cabals, "2")
    }

    func testNoCabalsIsZeroDollarsTwiceAndZero() {
        let stats = ProfileStats(.sampleEmpty)

        XCTAssertEqual(stats.inCabals, "$0.00")
        XCTAssertEqual(stats.returnDollars, "$0.00")
        XCTAssertEqual(stats.cabals, "0")
    }

    func testANullReturnShowsTheAmountAlone() {
        var portfolio = Components.Schemas.MyPortfolio.sample
        portfolio.returnBps = nil

        let stats = ProfileStats(portfolio)
        XCTAssertEqual(stats.returnDollars, "+$14.00")
        XCTAssertNil(stats.returnPercent)
    }

    func testALossUsesTheTypographicMinusOnBothFigures() {
        let stats = ProfileStats(.sampleLoss)

        XCTAssertEqual(stats.inCabals, "$949.00")
        XCTAssertEqual(stats.returnDollars, "\u{2212}$1.00")
        XCTAssertEqual(stats.returnPercent, "\u{2212}0.1%")
        XCTAssertFalse(stats.returnDollars.contains("-"))
        XCTAssertFalse(stats.returnPercent?.contains("-") ?? true)
    }

    func testCabalsCountsEveryEntryEvenOneWithNoValue() {
        XCTAssertEqual(ProfileStats(.sampleLoss).cabals, "2")
    }

    func testTheCountLabelIsSingularForOneCabalAndPluralOtherwise() {
        var portfolio = Components.Schemas.MyPortfolio.sample
        XCTAssertEqual(ProfileStats(portfolio).cabalsLabel, "Cabals")
        XCTAssertEqual(ProfileStats(.sampleEmpty).cabalsLabel, "Cabals")

        portfolio.cabals = [portfolio.cabals[0]]
        XCTAssertEqual(ProfileStats(portfolio).cabals, "1")
        XCTAssertEqual(ProfileStats(portfolio).cabalsLabel, "Cabal")
    }
}
