import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class AssetDetailClientModelTests: XCTestCase {
    func testLoadsTheGeneratedAssetDetail() async {
        let model = makeModel(
            StubTransport.Reply.json(.ok, detail),
            StubTransport.Reply.json(.ok, chart)
        )

        await model.load()

        XCTAssertEqual(model.phase, AssetDetailClientModel.Phase.loaded)
        XCTAssertEqual(model.detail?.name, "Apple")
        XCTAssertEqual(model.detail?.ticker, "AAPL")
        XCTAssertEqual(model.detail?.priceMicros, 110_000_000)
        XCTAssertEqual(model.detail?.changeBasisPoints, 1000)
        XCTAssertEqual(model.detail?.attribution, "Data provided by CoinGecko")
        XCTAssertEqual(model.detail?.otherListings.map(\.ticker), ["AAPL"])
        XCTAssertEqual(model.detail?.otherListings.map(\.name), ["Apple"])
        XCTAssertEqual(model.detail?.otherListings.first?.issuer, .prestocks)
        XCTAssertEqual(model.detail?.otherListings.first?.issuer.displayName, "PreStocks")
        XCTAssertEqual(model.chart?.range, .oneDay)
        XCTAssertEqual(model.chart?.points.map(\.priceUsdcMicros), [110_000_000, 111_000_000])
    }

    func testKeepsTheNewestChartRangeWhenAnEarlierRequestFinishesFirst() async {
        let transport = StubTransport(scripted: [.gate, .json(.ok, chart(range: "1Y"))])
        let api = APIClient(
            serverURL: testServerURL,
            tokens: StubTokenProvider(token: "token-1"),
            transport: transport
        )
        let model = AssetDetailClientModel(api: api, symbol: "AAPLx")

        let week = Task { await model.loadChart(range: .oneWeek) }
        while await transport.sent.count < 1 { await Task.yield() }
        let year = Task { await model.loadChart(range: .oneYear) }
        await year.value
        await transport.releaseGate(.json(.ok, chart(range: "1W")))
        await week.value
        await year.value

        XCTAssertEqual(model.selectedRange, .oneYear)
        XCTAssertEqual(model.chart?.range, .oneYear)
    }

    func testKeepsTheFailureForRetry() async {
        let model = makeModel(StubTransport.Reply.failure(URLError(.cannotConnectToHost)))

        await model.load()

        if case .failed = model.phase {} else { XCTFail("phase = \(model.phase)") }
        XCTAssertNotNil(model.lastError)
    }

    func testKeepsDetailWhenItsChartFails() async {
        let model = makeModel(
            StubTransport.Reply.json(.ok, detail),
            StubTransport.Reply.failure(URLError(.cannotLoadFromNetwork))
        )

        await model.load()

        XCTAssertEqual(model.phase, .loaded)
        XCTAssertEqual(model.detail?.ticker, "AAPL")
        XCTAssertNotNil(model.chartError)
    }

    func testSelectsEachChartRangeBeforeReadingIt() async {
        let model = makeModel(StubTransport.Reply.json(.ok, chart(range: "1Y")))

        await model.loadChart(range: .oneYear)

        XCTAssertEqual(model.selectedRange, .oneYear)
        XCTAssertEqual(model.chart?.range, .oneYear)
    }

    private func makeModel(_ responses: StubTransport.Reply...) -> AssetDetailClientModel {
        let api = APIClient(
            serverURL: testServerURL,
            tokens: StubTokenProvider(token: "token-1"),
            transport: StubTransport(scripted: responses)
        )
        return AssetDetailClientModel(api: api, symbol: "AAPLx")
    }

    private let detail = #"""
        {"symbol":"AAPLx","display_name":"Apple xStock","issuer":"xstocks","kind":"equity","logo_url":null,"price_micros":110000000,"price_as_of":null,"change_bps":1000,"sparkline_micros":[],"session":{"state":"open","continuous":false,"holiday":"","early_close":false,"next_state":"after_hours","next_transition":null},"decimals":8,"ui_multiplier":{"num":1,"den":1},"tradable":true,"other_listings":[{"symbol":"AAPLx","display_name":"Apple xStock","issuer":"prestocks","kind":"equity","logo_url":null,"tradable":false}],"attribution":"Data provided by CoinGecko"}
        """#

    private let chart = #"""
        {"range":"1D","bucket_seconds":300,"points":[{"t":"2026-03-04T14:30:00Z","open_micros":100000000,"high_micros":112000000,"low_micros":99000000,"close_micros":110000000},{"t":"2026-03-04T14:35:00Z","open_micros":110000000,"high_micros":113000000,"low_micros":109000000,"close_micros":111000000}],"empty":false,"attribution":"Data provided by CoinGecko"}
        """#

    private func chart(range: String) -> String {
        #"""
        {"range":"\#(range)","bucket_seconds":300,"points":[],"empty":true,"attribution":"Data provided by CoinGecko"}
        """#
    }
}
