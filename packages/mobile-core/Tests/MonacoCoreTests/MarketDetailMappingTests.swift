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
