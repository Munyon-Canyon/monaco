import MonacoAPI
import XCTest

@testable import MonacoCore

final class OneFigureTests: XCTestCase {
    private typealias Portfolio = Components.Schemas.MyPortfolio
    private typealias PortfolioCabal = Components.Schemas.PortfolioCabal
    private let start = Date(timeIntervalSince1970: 1_790_000_000)

    private func portfolio(
        total: Int64, arriving: Int64? = nil, cabals: [PortfolioCabal] = []
    ) -> Portfolio {
        Portfolio(
            cashMicros: 0, netWorthMicros: total, arrivingMicros: arriving, totalValueMicros: total, pnlMicros: 0,
            returnBps: nil, computedAt: nil, pricesAsOf: nil, cabals: cabals)
    }

    private func stake(
        value: Int64, shares: Int64 = 100, arriving: Int64? = nil, stale: Bool? = nil
    ) -> PortfolioCabal {
        PortfolioCabal(
            cabal: .sampleAlpha, valueMicros: value, shareUnits: shares, netContributedMicros: value,
            pnlMicros: 0, returnBps: nil, sliceBps: 10_000, ownershipBps: 3500, role: .member, canVote: true,
            arrivingMicros: arriving, stale: stale, potValueMicros: value, memberCount: 3, openVoteCount: 0)
    }

    private func cabal() -> Components.Schemas.MyCabal {
        Components.Schemas.MyCabal(
            id: Components.Schemas.CabalRef.sampleAlpha.id, name: "Alpha", pictureUrl: nil, role: "member",
            canVote: true, memberCount: 3, joinedAt: Date(timeIntervalSince1970: 0), pendingRequestCount: 0,
            unreadCount: 0)
    }

    func testProfileAndTheAllocationFloorTheSameFigure() {
        let held = portfolio(total: 1_237_500, cabals: [stake(value: 1_237_500)])
        let summary = PortfolioSummary(held)

        XCTAssertEqual(summary.stats.inCabals, "$1.23")
        XCTAssertEqual(summary.total, "$1.23")
        XCTAssertEqual(summary.rows.map(\.value), ["$1.23"])
        let profile = ProfileCabalRows.make(cabals: [cabal()], portfolio: held, invites: [])
        guard case .member(let row) = profile[0] else { return XCTFail("expected a member row") }
        XCTAssertEqual(row.value, "$1.23")
    }

    func testNetWorthCountsMoneyArrivingInACabal() {
        let held = portfolio(total: 1_000_000_000, arriving: 100_000_000)

        XCTAssertEqual(PortfolioSummary(held).netWorthMicros(cashMicros: 50_000_000), 1_150_000_000)
        XCTAssertEqual(PortfolioSummary(portfolio(total: 1_000_000_000)).netWorthMicros(cashMicros: 0), 1_000_000_000)
    }

    func testACashOutNotYetValuedTakesItsAmountOffTheCabalsAndNeverGoesBelowZero() {
        XCTAssertEqual(
            PortfolioSummary(portfolio(total: 1_000_000_000, arriving: -400_000_000))
                .netWorthMicros(cashMicros: 400_000_000),
            1_000_000_000)
        XCTAssertEqual(
            PortfolioSummary(portfolio(total: 100_000_000, arriving: -400_000_000)).netWorthMicros(cashMicros: 0), 0)
    }

    func testAFundedCabalNoRunHasValuedKeepsItsRowWithThePendingAmount() {
        let fresh = portfolio(
            total: 0, arriving: 100_000_000, cabals: [stake(value: 0, shares: 0, arriving: 100_000_000)])
        let rows = ProfileCabalRows.make(cabals: [cabal()], portfolio: fresh, invites: [])
        guard case .member(let row) = rows[0] else { return XCTFail("expected a member row") }

        XCTAssertEqual(row.value, "$100.00 pending")
    }

    func testAStakeWithMoneyArrivingReadsValuePlusWhatIsArriving() {
        let topped = portfolio(total: 1_000_000_000, cabals: [stake(value: 1_000_000_000, arriving: 100_000_000)])
        let rows = ProfileCabalRows.make(cabals: [cabal()], portfolio: topped, invites: [])
        guard case .member(let row) = rows[0] else { return XCTFail("expected a member row") }

        XCTAssertEqual(row.value, "$1,100.00")
        XCTAssertEqual(row.valueMicros, 1_100_000_000)
    }

    func testACabalTheLatestRunSkippedIsFlagged() {
        let summary = PortfolioSummary(portfolio(total: 1, cabals: [stake(value: 1, stale: true), stake(value: 1)]))

        XCTAssertEqual(summary.rows.map(\.isStale), [true, false])
        XCTAssertTrue(summary.hasStaleCabal)
    }

    private func curve(_ pairs: [(equity: Int64, pnl: Int64)]) -> ValueCurve {
        ValueCurve(
            samples: pairs.enumerated().map { index, pair in
                ValueCurve.Sample(
                    at: start.addingTimeInterval(TimeInterval(index * 3600)), value: pair.equity, pnl: pair.pnl)
            })
    }

    func testTheScrubPercentUsesTheHeroBaseOfStartPlusDeposits() {
        let deposited = curve([(100_000_000, 0), (1_010_000_000, 10_000_000)])

        XCTAssertEqual(deposited.change(.pnl)?.percentText, "+1.00%")
        XCTAssertEqual(deposited.change(.pnl)?.amountText, "+$10.00")
    }

    func testACashOutDoesNotShrinkTheScrubBase() {
        let cashedOut = curve([(1_000_000_000, 0), (500_000_000, 0), (510_000_000, 10_000_000)])

        XCTAssertEqual(cashedOut.change(.pnl)?.percentText, "+1.00%")
        XCTAssertEqual(cashedOut.change(.pnl, to: 1)?.percentText, "0.00%")
    }

    func testADepositAfterTheScrubbedPointIsNotInItsBase() {
        let later = curve([(100_000_000, 0), (101_000_000, 1_000_000), (1_011_000_000, 11_000_000)])

        XCTAssertEqual(later.change(.pnl, to: 1)?.percentText, "+1.00%")
        XCTAssertEqual(later.change(.pnl, to: 2)?.percentText, "+1.10%")
    }

    func testABaseUnderACentHasNoScrubPercent() {
        let tiny = curve([(0, 0), (5_000, 5_000)])

        XCTAssertNil(tiny.change(.pnl)?.percentText)
    }

    func testAHistoryWithNoReturnHasNoPercentInsteadOfZero() {
        let unknown = Components.Schemas.MyPnlHistory(
            range: ._1d, pnlMicros: 5_000_000, returnBps: 0, returnKnown: false, points: [])
        XCTAssertNil(ValueCurve(unknown).rangeReturnBps)
        XCTAssertEqual(ValueCurve(unknown).rangePnlMicros, 5_000_000)

        let known = Components.Schemas.MyPnlHistory(
            range: ._1d, pnlMicros: 0, returnBps: 0, returnKnown: true, points: [])
        XCTAssertEqual(ValueCurve(known).rangeReturnBps, 0)
        let older = Components.Schemas.MyPnlHistory(range: ._1d, pnlMicros: 0, returnBps: 0, points: [])
        XCTAssertEqual(ValueCurve(older).rangeReturnBps, 0)
    }

    func testAFreshFundMakesHomeInvestedAndTheRowFunded() {
        let fresh = PortfolioSummary(
            portfolio(total: 0, arriving: 100_000_000, cabals: [stake(value: 0, shares: 0, arriving: 100_000_000)]))

        XCTAssertEqual(HomeVariant(summary: fresh, cashMicros: 0), .invested)
        let row = HomeCabalRow(fresh.rows[0])
        XCTAssertEqual(row.valueMicros, 100_000_000)
        XCTAssertTrue(row.isFunded)
        let none = PortfolioSummary(portfolio(total: 0, cabals: [stake(value: 0, shares: 0)]))
        XCTAssertEqual(HomeVariant(summary: none, cashMicros: 5), .cashOnly)
    }
}
