import Foundation
import XCTest

@testable import MonacoCore

final class SharePositionBuilderTests: XCTestCase {
    private let utc = TimeZone(identifier: "UTC") ?? .gmt
    private var now: Date { Self.date(2026, 10, 22) }

    private static func date(_ year: Int, _ month: Int, _ day: Int) -> Date {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = TimeZone(identifier: "UTC") ?? .gmt
        return calendar.date(from: DateComponents(year: year, month: month, day: day, hour: 15)) ?? .distantPast
    }

    private var crest: ShareCrest { ShareCrest(cabalID: "cabal-1", name: "AC Capital", pictureURL: nil) }

    private var position: SharePosition {
        SharePosition(
            crest: crest, symbol: "NVDA", logoURL: nil, unitsText: "38.0000", valueMicros: 8_360_000_000,
            costBasisMicros: 7_332_100_000, pnlMicros: 1_027_900_000, avgEntryMicros: 192_950_000,
            priceMicros: 220_000_000, firstBoughtAt: Self.date(2026, 6, 3))
    }

    private var trades: [ShareTrade] {
        [
            ShareTrade(isBuy: true, time: Self.date(2026, 6, 3), priceMicros: 181_200_000),
            ShareTrade(isBuy: true, time: Self.date(2026, 7, 14), priceMicros: 188_400_000),
            ShareTrade(isBuy: false, time: Self.date(2026, 8, 20), priceMicros: 214_100_000),
            ShareTrade(isBuy: true, time: Self.date(2026, 9, 30), priceMicros: 196_750_000),
        ]
    }

    private var chart: [ShareChartPoint] {
        [
            ShareChartPoint(time: Self.date(2026, 5, 20), priceMicros: 170_000_000),
            ShareChartPoint(time: Self.date(2026, 6, 3), priceMicros: 181_200_000),
            ShareChartPoint(time: Self.date(2026, 8, 20), priceMicros: 214_100_000),
            ShareChartPoint(time: Self.date(2026, 10, 22), priceMicros: 220_000_000),
        ]
    }

    private func build(
        position: SharePosition? = nil, trades: [ShareTrade]? = nil, code: String? = "andres",
        eyes: ShareEyes = ShareEyes(), truncated: Bool = false
    ) -> ShareCardSpec {
        SharePositionBuilder.build(
            position: position ?? self.position, trades: trades ?? self.trades, chart: chart, code: code, eyes: eyes,
            truncated: truncated, now: now, timeZone: utc)
    }

    func testTheFrameBuildsFromTheFixtures() {
        let spec = build()

        XCTAssertEqual(spec.header.title, "AC Capital")
        XCTAssertEqual(spec.header.subtitle, "Holding · 4 calls")
        XCTAssertEqual(spec.header.dateText, "Oct 22, 2026")
        XCTAssertEqual(spec.tickerLine.symbol, "NVDA")
        XCTAssertEqual(spec.tickerLine.detail, "Long · 38 shares")
        XCTAssertEqual(spec.figure.main, "+$1,027.90")
        XCTAssertEqual(spec.figure.secondary, "+14.0%")
        XCTAssertEqual(spec.figure.tone, .gain)
        XCTAssertEqual(spec.stats.map(\.label), ["Size", "Avg. entry", "Current"])
        XCTAssertEqual(spec.stats.map(\.value), ["$8,360", "$192.95", "$220.00"])
        XCTAssertEqual(spec.code, "ANDRES")
        XCTAssertEqual(spec.facts, ShareFacts(symbol: "NVDA", firstBoughtAt: Self.date(2026, 6, 3), side: .long))
        XCTAssertEqual(spec.spoken, "AC Capital, NVDA, long 38 shares, up 1,027 dollars 90, 14 percent")
    }

    func testEveryTradeIsAMarkerAtItsPriceAndDate() {
        let markers = build().chart.markers

        XCTAssertEqual(markers.map(\.kind), [.buy, .buy, .sell, .buy])
        XCTAssertEqual(markers.map(\.priceMicros), [181_200_000, 188_400_000, 214_100_000, 196_750_000])
        XCTAssertEqual(markers.map(\.time), trades.map(\.time))
    }

    func testOnlyTheLatestSellAndTheBuyAfterItCarryCallouts() {
        let spec = build()

        XCTAssertEqual(spec.chart.markers.map(\.callout), [nil, nil, "Aug 20 · $214.10", "Sep 30 · $196.75"])
        XCTAssertEqual(spec.chart.callouts.count, 2)
    }

    func testABuyOnlyHistoryCallsOutTheLatestBuy() {
        let spec = build(trades: Array(trades.prefix(2)))

        XCTAssertEqual(spec.chart.markers.map(\.callout), [nil, "Jul 14 · $188.40"])
    }

    func testASellOnlyHistoryCallsOutTheSell() {
        let spec = build(trades: [trades[2]])

        XCTAssertEqual(spec.chart.markers.map(\.callout), ["Aug 20 · $214.10"])
        XCTAssertEqual(spec.header.subtitle, "Holding · 1 call")
    }

    func testASingleBuyIsOneMarkerWithACallout() {
        let spec = build(trades: [trades[0]])

        XCTAssertEqual(spec.chart.markers.count, 1)
        XCTAssertEqual(spec.chart.markers.first?.callout, "Jun 3 · $181.20")
    }

    func testTheChartStartsAtTheFirstBuy() {
        let points = build().chart.points

        XCTAssertEqual(points.first?.time, Self.date(2026, 6, 3))
        XCTAssertEqual(points.count, 3)
    }

    func testATruncatedHistoryReadsAsAtLeastAndStartsAtTheOldestLoadedTrade() {
        let spec = build(trades: Array(trades.suffix(2)), truncated: true)

        XCTAssertEqual(spec.header.subtitle, "Holding · 2+ calls")
        XCTAssertEqual(spec.chart.points.first?.time, Self.date(2026, 8, 20))
    }

    func testPnlOffMakesThePercentTheFigureAndDropsEveryDollarOfTheUsersMoney() {
        let spec = build(eyes: ShareEyes(pnlDollars: false))

        XCTAssertEqual(spec.figure.main, "+14.0%")
        XCTAssertNil(spec.figure.secondary)
        XCTAssertEqual(spec.stats.map(\.label), ["Shares", "Avg. entry", "Current"])
        XCTAssertEqual(spec.stats.first?.value, "38")
        XCTAssertEqual(spec.spoken, "AC Capital, NVDA, long 38 shares, up 14 percent")
    }

    func testBuySellOffDropsMarkersAndCallouts() {
        let spec = build(eyes: ShareEyes(buySell: false))

        XCTAssertTrue(spec.chart.markers.isEmpty)
        XCTAssertTrue(spec.chart.callouts.isEmpty)
        XCTAssertEqual(spec.header.subtitle, "Holding · 4 calls")
    }

    func testCodeOffLeavesTheFooterAsTheWordmark() {
        XCTAssertNil(build(eyes: ShareEyes(code: false)).code)
        XCTAssertNil(build(code: nil).code)
    }

    func testALossReadsNegativeAndDown() {
        let losing = SharePosition(
            crest: crest, symbol: "NVDA", logoURL: nil, unitsText: "1.0000", valueMicros: 90_000_000,
            costBasisMicros: 100_000_000, pnlMicros: -10_000_000, avgEntryMicros: 100_000_000,
            priceMicros: 90_000_000, firstBoughtAt: nil)
        let spec = build(position: losing)

        XCTAssertEqual(spec.figure.main, "−$10.00")
        XCTAssertEqual(spec.figure.secondary, "−10.0%")
        XCTAssertEqual(spec.figure.tone, .loss)
        XCTAssertEqual(spec.tickerLine.detail, "Long · 1 share")
        XCTAssertEqual(spec.facts.firstBoughtAt, nil)
        XCTAssertEqual(spec.spoken, "AC Capital, NVDA, long 1 share, down 10 dollars, 10 percent")
    }
}
