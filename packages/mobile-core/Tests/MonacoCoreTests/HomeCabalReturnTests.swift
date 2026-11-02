import MonacoAPI
import XCTest

@testable import MonacoCore

final class HomeCabalReturnTests: XCTestCase {
    private func row(returnBps: Int64?) -> HomeCabalRow {
        let stake = Components.Schemas.PortfolioCabal(
            cabal: .sampleAlpha, valueMicros: 100_000_000, shareUnits: 100, netContributedMicros: 100_000_000,
            pnlMicros: 4_700_000, returnBps: returnBps, sliceBps: 10_000, ownershipBps: 3500, role: .member,
            canVote: true, arrivingMicros: nil, stale: nil, potValueMicros: 100_000_000, memberCount: 3,
            openVoteCount: 0)
        let portfolio = Components.Schemas.MyPortfolio(
            cashMicros: 0, netWorthMicros: 100_000_000, arrivingMicros: nil, totalValueMicros: 100_000_000,
            pnlMicros: 0, returnBps: nil, computedAt: nil, pricesAsOf: nil, cabals: [stake])
        return HomeCabalRow(PortfolioSummary(portfolio).rows[0])
    }

    func testHomeReadsTheSameOneDecimalReturnAsProfileAndCabals() {
        XCTAssertEqual(row(returnBps: 470).returnText, "+4.7%")
        XCTAssertEqual(row(returnBps: -170).returnText, "\u{2212}1.7%")
        XCTAssertEqual(row(returnBps: 1120).returnText, "+11.2%")
    }

    func testNoReturnReadsNoFigure() {
        XCTAssertNil(row(returnBps: nil).returnText)
    }
}
