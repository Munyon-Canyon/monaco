import Foundation
import MonacoAPI
import MonacoCore
import XCTest

final class CabalHoldingTests: XCTestCase {
    private let now = Date(timeIntervalSince1970: 1_790_996_400)

    func testTheReadyFixtureReadsLikeTheReference() {
        let holding = CabalHoldingFixture.holding(.ready, now: now)

        XCTAssertEqual(holding.valueText, "$6,600.00")
        XCTAssertEqual(holding.returnText(for: .oneMonth), "+$525.00")
        XCTAssertEqual(holding.returnTone(for: .oneMonth), .gain)
        XCTAssertEqual(holding.header, "NVDA · Big Ballers")
        XCTAssertEqual(holding.headerSpoken, "NVIDIA, NVDA, held by Big Ballers")
        XCTAssertEqual(
            holding.stats.map(\.value),
            ["30.0000", "$206.43", "$230.00", "$6,192.86", "+$117.86", "+$407.14"])
        XCTAssertEqual(holding.stats.map(\.label).first, "Units")
        XCTAssertFalse(holding.hasMissingFigures)
    }

    func testTheLossFixtureTurnsTheReturnAndTheUnrealizedCellNegative() {
        let holding = CabalHoldingFixture.holding(.loss, now: now)

        XCTAssertEqual(holding.valueText, "$5,412.00")
        XCTAssertEqual(holding.returnText(for: .oneMonth), "−$663.00")
        XCTAssertEqual(holding.returnTone(for: .oneMonth), .loss)
        XCTAssertEqual(holding.stats.last?.tone, .loss)
    }

    func testAnEmptyPositionReadsAsZeroWithAGainToneAndDashes() {
        let holding = CabalHoldingFixture.holding(.empty, now: now)

        XCTAssertTrue(holding.isEmpty)
        XCTAssertEqual(holding.returnText(for: .oneMonth), "$0.00")
        XCTAssertEqual(holding.returnTone(for: .oneMonth), .gain)
        XCTAssertEqual(holding.stats.map(\.tone), [.neutral, .missing, .missing, .missing, .missing, .missing])
    }

    func testAMissingFigureIsSpokenAsNotAvailable() {
        let holding = CabalHoldingFixture.holding(.productionGaps, now: now)

        XCTAssertEqual(holding.stats[1].spoken, "Average buy, not available yet")
        XCTAssertEqual(holding.stats[0].spoken, "Units, 30.0000")
    }

    func testEveryPeriodHasASeriesInTheFixtureAndEndsAtThePositionValue() throws {
        let holding = CabalHoldingFixture.holding(.ready, now: now)

        for period in HoldingPeriod.allCases {
            let curve = try XCTUnwrap(holding.curve(for: period))
            XCTAssertEqual(curve.samples.last?.value, holding.valueMicros)
        }
    }

    func testPeriodsMapOntoTheChartRanges() {
        XCTAssertEqual(HoldingPeriod.allCases.map(\.label), ["1W", "1M", "3M", "1Y", "All"])
        XCTAssertEqual(HoldingPeriod.defaultPeriod, .oneMonth)
        XCTAssertEqual(HoldingPeriod.threeMonths.chartRange, .oneMonth)
        XCTAssertEqual(HoldingPeriod.oneYear.chartRange, .all)
    }

    func testTheCopyIsExactAndPassesTheMainFlowAudit() {
        XCTAssertEqual(CabalHoldingCopy.title("NVIDIA"), "NVIDIA holding")
        XCTAssertEqual(CabalHoldingCopy.stale(14), "Holding value last updated 14 minutes ago.")
        XCTAssertEqual(CabalHoldingCopy.stale(1), "Holding value last updated 1 minute ago.")
        XCTAssertEqual(CabalHoldingCopy.utc("Big Ballers"), "Big Ballers · Times shown in UTC")
        XCTAssertEqual(
            CabalHoldingCopy.privateBody("Big Ballers"), "Only members of Big Ballers can see its positions.")
        XCTAssertTrue(
            MainFlowCopyAudit.stringsAreClean([
                CabalHoldingCopy.fallbackTitle, CabalHoldingCopy.positionValue, CabalHoldingCopy.noSales,
                CabalHoldingCopy.emptyTitle, CabalHoldingCopy.emptyBody,
                CabalHoldingCopy.tradeHistory, CabalHoldingCopy.noTrades, CabalHoldingCopy.olderTrades,
                CabalHoldingCopy.tradeFailed, CabalHoldingCopy.related, CabalHoldingCopy.details,
                CabalHoldingCopy.viewMarket, CabalHoldingCopy.errorTitle, CabalHoldingCopy.errorBody,
                CabalHoldingCopy.refreshFailed, CabalHoldingCopy.privateTitle,
                CabalHoldingCopy.privateBody("Big Ballers"), CabalHoldingCopy.noProposals("NVDA"),
            ]))
    }
}

final class HoldingTradeTests: XCTestCase {
    private typealias Activity = Components.Schemas.CabalActivity

    func testABuyShowsUnitsDatePriceAndTotal() throws {
        let activity = Activity(
            id: "t1", kind: .buy, status: .confirmed, asset: .init(symbol: "NVDAx", name: "NVIDIA"),
            usdcMicros: 4_888_880_000, units: 2_444_440_000, actor: nil, txSignature: nil,
            occurredAt: Date(timeIntervalSince1970: 1_790_245_080))

        let trade = try XCTUnwrap(HoldingTrade(activity, symbol: "NVDAx", decimals: 8))

        XCTAssertEqual(trade.title, "Buy · 24.4444 units")
        XCTAssertEqual(trade.unitPriceText, "$200.00")
        XCTAssertEqual(trade.priceLine, "$200.00 per unit")
        XCTAssertEqual(trade.totalText, "$4,888.88")
        XCTAssertEqual(trade.dateText, "Sep 24, 2026 · 10:18 UTC")
        XCTAssertEqual(
            trade.spoken, "Buy, 24.4444 units, Sep 24, 2026 · 10:18 UTC, $200.00 per unit, total $4,888.88")
    }

    func testASellReadsAsASell() throws {
        let activity = Activity(
            id: "t2", kind: .sell, status: .confirmed, asset: .init(symbol: "NVDAx", name: "NVIDIA"),
            usdcMicros: 1_840_000_000, units: 800_000_000, actor: nil, txSignature: nil,
            occurredAt: Date(timeIntervalSince1970: 1_790_245_080))

        let trade = try XCTUnwrap(HoldingTrade(activity, symbol: "NVDAx", decimals: 8))

        XCTAssertFalse(trade.isBuy)
        XCTAssertEqual(trade.title, "Sell · 8.0000 units")
        XCTAssertEqual(trade.unitPriceText, "$230.00")
    }

    func testRowsThatAreNotConfirmedTradesOfTheSymbolAreDropped() {
        let asset = Activity.AssetPayload(symbol: "NVDAx", name: "NVIDIA")
        let date = Date(timeIntervalSince1970: 1_790_245_080)
        let pending = Activity(
            id: "a", kind: .buy, status: .pending, asset: asset, usdcMicros: 1, units: 1, actor: nil,
            txSignature: nil, occurredAt: date)
        let other = Activity(
            id: "b", kind: .buy, status: .confirmed, asset: .init(symbol: "AAPLx", name: "Apple"), usdcMicros: 1,
            units: 1, actor: nil, txSignature: nil, occurredAt: date)
        let fund = Activity(
            id: "c", kind: .fund, status: .confirmed, asset: asset, usdcMicros: 1, units: 1, actor: nil,
            txSignature: nil, occurredAt: date)
        let unknown = Activity(
            id: "d", kind: .buy, status: .confirmed, asset: asset, usdcMicros: nil, units: nil, actor: nil,
            txSignature: nil, occurredAt: date)

        for activity in [pending, other, fund, unknown] {
            XCTAssertNil(HoldingTrade(activity, symbol: "NVDAx", decimals: 8))
        }
    }
}
