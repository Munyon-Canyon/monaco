import Foundation
import MonacoAPI
import MonacoCore
import XCTest

final class MarketDetailMappingTests: XCTestCase {
    private let sampledAt = Date(timeIntervalSince1970: 1_772_596_200)

    func testChartMapsItsRangeAndCandles() {
        let chart = MarketMapping.chart(
            .init(
                range: ._1d,
                interval: ._5m,
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
                attribution: "Data provided by CoinGecko",
                nextBefore: nil
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

    func testDetailMapsTheVolumeWhenTheBackendSendsOne() {
        var wire = Components.Schemas.AssetDetail.googl
        wire.volume24hMicros = 1_500_000_500_000
        wire.volume24hAsOf = sampledAt

        XCTAssertEqual(MarketMapping.detail(wire).asset.volume24hMicros, 1_500_000_500_000)

        wire.volume24hMicros = nil
        wire.volume24hAsOf = nil

        XCTAssertNil(MarketMapping.detail(wire).asset.volume24hMicros)
    }

    func testChartMapsThePreviousCloseWhenTheBackendSendsIt() {
        let chart = MarketMapping.chart(chartWire(previousCloseMicros: 108_000_000))

        XCTAssertEqual(chart.previousCloseUsdcMicros, 108_000_000)
        XCTAssertTrue(chart.drawsBaselineRule)
    }

    func testChartWithoutAPreviousCloseDrawsNoRule() {
        let chart = MarketMapping.chart(chartWire(previousCloseMicros: nil))

        XCTAssertNil(chart.previousCloseUsdcMicros)
        XCTAssertFalse(chart.drawsBaselineRule)
    }

    private func chartWire(previousCloseMicros: Int64?) -> Components.Schemas.AssetChart {
        .init(
            range: ._1d,
            interval: ._5m,
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
            attribution: "Data provided by CoinGecko",
            nextBefore: nil,
            previousCloseMicros: previousCloseMicros
        )
    }
}
