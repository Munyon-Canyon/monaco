import MonacoAPI
import XCTest

@testable import MonacoCore

final class PortfolioSummaryTests: XCTestCase {
    func testTheHeroTotalAndChipAreFormattedFromServerFigures() {
        let summary = PortfolioSummary(.sample)

        XCTAssertEqual(summary.total, "$1,000.00")
        XCTAssertEqual(summary.chip, "▲ $14.00 · 1.4%")
        XCTAssertEqual(summary.pnl, "+$14.00")
        XCTAssertEqual(summary.returnText, "+1.4%")
        XCTAssertEqual(summary.direction, .up)
        XCTAssertFalse(summary.isEmpty)
    }

    func testRowsCarryValueGainReturnAndSliceInServerOrder() {
        let rows = PortfolioSummary(.sample).rows

        XCTAssertEqual(rows.map(\.name), ["Alpha", "Sunday Investors"])
        XCTAssertEqual(rows[0].value, "$600.00")
        XCTAssertEqual(rows[0].pnl, "+$10.00")
        XCTAssertEqual(rows[0].returnText, "+1.7%")
        XCTAssertEqual(rows[0].slice, "60.00%")
        XCTAssertEqual(rows[1].slice, "40.00%")
        XCTAssertEqual(rows[0].share, "60% of your cabals")
        XCTAssertEqual(rows[1].share, "40% of your cabals")
        XCTAssertEqual(rows[0].id, Components.Schemas.CabalRef.sampleAlpha.id)
    }

    func testASingleCabalHasNoShareSubtitle() {
        var portfolio = Components.Schemas.MyPortfolio.sample
        portfolio.cabals = Array(portfolio.cabals.prefix(1))

        XCTAssertNil(PortfolioSummary(portfolio).rows[0].share)
    }

    func testANullReturnShowsADashAndAZeroSliceShowsZero() {
        let rows = PortfolioSummary(.sampleLoss).rows

        XCTAssertEqual(rows[1].returnText, "—")
        XCTAssertEqual(rows[1].slice, "0.00%")
        XCTAssertEqual(rows[1].pnl, "$0.00")
        XCTAssertEqual(rows[1].direction, .flat)
    }

    func testALossPointsDownAndTheChipReadsAsMagnitudes() {
        let summary = PortfolioSummary(.sampleLoss)

        XCTAssertEqual(summary.direction, .down)
        XCTAssertEqual(summary.chip, "▼ $1.00 · 0.1%")
        XCTAssertEqual(summary.rows[0].pnl, "−$1.00")
        XCTAssertEqual(summary.rows[0].returnText, "−0.1%")
    }

    func testNoCabalsIsAZeroDollarChipWithNoPercent() {
        let summary = PortfolioSummary(.sampleEmpty)

        XCTAssertTrue(summary.isEmpty)
        XCTAssertEqual(summary.total, "$0.00")
        XCTAssertEqual(summary.chip, "$0.00")
        XCTAssertEqual(summary.direction, .flat)
    }

    func testAPortfolioWithCabalsButNoReturnYetShowsADash() {
        var portfolio = Components.Schemas.MyPortfolio.sample
        portfolio.returnBps = nil

        XCTAssertEqual(PortfolioSummary(portfolio).chip, "▲ $14.00 · —")
        XCTAssertEqual(PortfolioSummary(portfolio).returnText, "—")
    }
}
