import Foundation
import MonacoAPI
import MonacoCore
import XCTest

final class MarketDetailMappingTests: XCTestCase {
    private let sampledAt = Date(timeIntervalSince1970: 1_772_596_200)

    func testPreIPOWithSiblingListingsMapsTheDetailWithoutLosingItsCatalogFields() {
        let detail = MarketMapping.detail(
            .init(
                symbol: "tSpaceX",
                displayName: "T-SpaceX",
                issuer: .tessera,
                kind: .preIpo,
                logoUrl: nil,
                priceMicros: 42_000_000,
                priceAsOf: sampledAt,
                changeBps: 125,
                sparklineMicros: [40_000_000, 42_000_000],
                session: .init(state: .open, continuous: true, holiday: "", earlyClose: false),
                decimals: 6,
                uiMultiplier: .init(num: 1, den: 1),
                tradable: true,
                otherListings: [
                    .init(
                        symbol: "SPACEX",
                        displayName: "SpaceX",
                        issuer: .prestocks,
                        kind: .preIpo,
                        tradable: false
                    )
                ],
                attribution: "Data provided by CoinGecko"
            )
        )

        XCTAssertEqual(detail.asset.symbol, "tSpaceX")
        XCTAssertEqual(detail.asset.ticker, "tSpaceX")
        XCTAssertEqual(detail.asset.name, "SpaceX")
        XCTAssertEqual(detail.asset.issuer, .tessera)
        XCTAssertEqual(detail.asset.kind, .preIpo)
        XCTAssertEqual(detail.asset.priceText, "$42.00")
        XCTAssertEqual(detail.asset.changeText, "+1.25%")
        XCTAssertEqual(detail.asset.sparkline?.lastUsdcMicros, 42_000_000)
        XCTAssertFalse(detail.asset.showsSessionChip)
        XCTAssertEqual(detail.otherListings.count, 1)
        XCTAssertEqual(detail.otherListings[0].symbol, "SPACEX")
        XCTAssertEqual(detail.otherListings[0].name, "SpaceX")
        XCTAssertEqual(detail.otherListings[0].ticker, "SPACEX")
        XCTAssertEqual(detail.otherListings[0].issuer, .prestocks)
        XCTAssertEqual(detail.otherListings[0].kind, .preIpo)
        XCTAssertFalse(detail.otherListings[0].isTradable)
    }

    func testChartMapsItsRangeAndCandles() {
        let chart = MarketMapping.chart(
            .init(
                range: ._1d,
                bucketSeconds: 300,
                points: [
                    .init(
                        t: sampledAt,
                        openMicros: 100_000_000,
                        highMicros: 112_000_000,
                        lowMicros: 99_000_000,
                        closeMicros: 110_000_000
                    )
                ],
                empty: false,
                attribution: "Data provided by CoinGecko"
            )
        )

        XCTAssertEqual(chart.range, .oneDay)
        XCTAssertEqual(chart.points.count, 1)
        XCTAssertEqual(chart.points[0].timestamp, Int64(sampledAt.timeIntervalSince1970))
        XCTAssertEqual(chart.points[0].priceUsdcMicros, 110_000_000)
        XCTAssertEqual(chart.points[0].openUsdcMicros, 100_000_000)
        XCTAssertEqual(chart.points[0].highUsdcMicros, 112_000_000)
        XCTAssertEqual(chart.points[0].lowUsdcMicros, 99_000_000)
    }
}
