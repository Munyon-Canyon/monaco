import MonacoAPI
import XCTest

final class FeedItemChartSampleTests: XCTestCase {
    func testTheDegensResultCarriesARisingChartWithoutMarkers() throws {
        let item = try XCTUnwrap(Components.Schemas.FeedItem.results.first { $0.assetName == "META" })
        let chart = try XCTUnwrap(item.chart)
        XCTAssertEqual(item.returnBps, 109)
        XCTAssertEqual(chart.points.count, 32)
        XCTAssertEqual(chart.points.first?.priceMicros, 716_520_000)
        XCTAssertEqual(chart.points.last?.priceMicros, 724_400_000)
        let prices = chart.points.map { $0.priceMicros }
        XCTAssertEqual(prices, prices.sorted())
        XCTAssertTrue(chart.markers.isEmpty)
    }

    func testTheTechTitansSaleMarksTheBuyBeforeTheSell() throws {
        let item = try XCTUnwrap(Components.Schemas.FeedItem.results.first { $0.assetName == "AAPL" })
        let chart = try XCTUnwrap(item.chart)
        XCTAssertEqual(item.returnBps, 670)
        XCTAssertEqual(chart.markers.map { $0.side }, [.buy, .sell])
    }
}
