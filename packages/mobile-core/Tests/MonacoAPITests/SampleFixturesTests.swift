import Foundation
import XCTest

@testable import MonacoAPI

final class SampleFixturesTests: XCTestCase {
    func testTheChartSamplesDescribeOneDayAnEmptyDayAndGeneratedCandles() {
        let oneDay = Components.Schemas.AssetChart.oneDay
        XCTAssertEqual(oneDay.points.count, 1)
        XCTAssertFalse(oneDay.empty)
        XCTAssertEqual(oneDay.bucketSeconds, 300)

        let empty = Components.Schemas.AssetChart.empty
        XCTAssertTrue(empty.points.isEmpty)
        XCTAssertTrue(empty.empty)

        let candles = Components.Schemas.AssetChart.candles
        XCTAssertEqual(candles.points.count, 300)
        XCTAssertEqual(candles.bucketSeconds, 300)
        XCTAssertEqual(candles.previousCloseMicros, 170_400_000)
        XCTAssertFalse(candles.empty)
        for (earlier, later) in zip(candles.points, candles.points.dropFirst()) {
            XCTAssertEqual(later.t.timeIntervalSince(earlier.t), 300)
            XCTAssertGreaterThanOrEqual(earlier.highMicros, max(earlier.openMicros, earlier.closeMicros))
            XCTAssertLessThanOrEqual(earlier.lowMicros, min(earlier.openMicros, earlier.closeMicros))
            XCTAssertEqual(later.openMicros, earlier.closeMicros)
        }

        let sparse = Components.Schemas.AssetChart.candles(range: ._1d, interval: ._1h, count: 6, sparse: true)
        XCTAssertEqual(sparse.points.count, 6)
        XCTAssertEqual(sparse.bucketSeconds, 3_600)
        for (index, point) in sparse.points.enumerated() where index % 3 != 0 {
            XCTAssertEqual(point.openMicros, point.closeMicros, "a sparse bar off the beat is flat")
            XCTAssertEqual(point.highMicros, point.closeMicros)
            XCTAssertEqual(point.lowMicros, point.closeMicros)
        }

        let widths: [(Components.Schemas.AssetChart.IntervalPayload, Int64)] = [
            (._1m, 60), (._5m, 300), (._15m, 900), (._30m, 1_800), (._1h, 3_600), (._4h, 14_400), (._1d, 86_400),
            (._1w, 604_800),
        ]
        for (interval, width) in widths {
            let chart = Components.Schemas.AssetChart.candles(range: ._1d, interval: interval, count: 2)
            XCTAssertEqual(chart.bucketSeconds, width, "\(interval)")
            XCTAssertEqual(chart.points[1].t.timeIntervalSince(chart.points[0].t), TimeInterval(width))
        }
    }

    func testTheAssetListSamplesEachNameAtLeastOneAsset() {
        XCTAssertEqual(Components.Schemas.AssetList.popularSample.nextCursor, "popular-next")
        XCTAssertNil(Components.Schemas.AssetList.preIpoSample.nextCursor)
        for (name, list) in [
            ("popular", Components.Schemas.AssetList.popularSample),
            ("preIpo", Components.Schemas.AssetList.preIpoSample),
            ("movers", Components.Schemas.AssetList.moversSample),
            ("inCabals", Components.Schemas.AssetList.inCabalsSample),
            ("all", Components.Schemas.AssetList.allSample),
            ("nameOrdered", Components.Schemas.AssetList.nameOrderedSample),
            ("priceOrdered", Components.Schemas.AssetList.priceOrderedSample),
        ] {
            XCTAssertFalse(list.assets.isEmpty, name)
        }
    }
}
