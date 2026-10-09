import MonacoCore
import XCTest

final class AssetStatsTests: XCTestCase {
    private func candle(_ t: Int64, o: Int64, h: Int64, l: Int64, c: Int64) -> MarketChartPoint {
        MarketChartPoint(timestamp: t, priceUsdcMicros: c, openUsdcMicros: o, highUsdcMicros: h, lowUsdcMicros: l)
    }

    func testReadsOpenAndDayRangeFromTheDayCandles() {
        let day = AssetChartSeries(
            range: .oneDay,
            points: [
                candle(1, o: 100_000_000, h: 103_000_000, l: 99_000_000, c: 101_000_000),
                candle(2, o: 101_000_000, h: 105_000_000, l: 100_000_000, c: 104_000_000),
            ])
        let stats = AssetStats(day: day, year: nil, priceMicros: 104_000_000, changeBasisPoints: nil)

        XCTAssertEqual(stats.open, 100_000_000)
        XCTAssertEqual(stats.dayHigh, 105_000_000)
        XCTAssertEqual(stats.dayLow, 99_000_000)
        XCTAssertNil(stats.yearHigh)
        XCTAssertNil(stats.previousClose)
    }

    func testDerivesThePreviousCloseFromTheQuoteAndItsMove() {
        let stats = AssetStats(day: nil, year: nil, priceMicros: 110_000_000, changeBasisPoints: 1000)
        XCTAssertEqual(stats.previousClose, 100_000_000)
    }

    func testPlacesThePriceInTheYearRangeAndClampsIt() {
        let year = AssetChartSeries(
            range: .oneYear,
            points: [
                candle(1, o: 100_000_000, h: 150_000_000, l: 90_000_000, c: 120_000_000),
                candle(2, o: 120_000_000, h: 130_000_000, l: 110_000_000, c: 125_000_000),
            ])
        let stats = AssetStats(day: nil, year: year, priceMicros: nil, changeBasisPoints: nil)

        XCTAssertEqual(stats.yearLow, 90_000_000)
        XCTAssertEqual(stats.yearHigh, 150_000_000)
        XCTAssertEqual(stats.yearRangeBasisPoints(priceMicros: 120_000_000), 5000)
        XCTAssertEqual(stats.yearRangeBasisPoints(priceMicros: 200_000_000), 10_000)
        XCTAssertEqual(stats.yearRangeBasisPoints(priceMicros: 50_000_000), 0)
        XCTAssertNil(stats.yearRangeBasisPoints(priceMicros: nil))
    }

    func testPositionReturnIsPnlOverCostBasis() {
        let position = AssetCabalPosition(
            cabalID: "c", cabalName: "N", pictureURL: nil, canVote: true, units: "1.0000",
            kind: .stock, valueMicros: 120_000_000, pnlMicros: 20_000_000, costBasisMicros: 100_000_000)
        XCTAssertEqual(position.returnBasisPoints, 2000)
    }

    func testPositionLabelFollowsTheAssetKind() {
        func label(_ kind: AssetKind) -> String {
            AssetCabalPosition(
                cabalID: "c", cabalName: "N", pictureURL: nil, canVote: true, units: "2.5000", kind: kind,
                valueMicros: 0, pnlMicros: 0, costBasisMicros: 0
            ).sharesLabel
        }
        XCTAssertEqual(label(.stock), "2.5 shares")
        XCTAssertEqual(label(.preIpo), "2.5 tokens")
    }
}
