import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class AssetDetailClientModelTests: XCTestCase {
    func testLoadsTheGeneratedAssetDetail() async {
        let model = makeModel(StubTransport.Reply.json(.ok, detail))

        await model.load()

        XCTAssertEqual(model.phase, AssetDetailClientModel.Phase.loaded)
        XCTAssertEqual(model.detail?.name, "Apple")
        XCTAssertEqual(model.detail?.ticker, "AAPL")
        XCTAssertEqual(model.detail?.priceMicros, 110_000_000)
        XCTAssertEqual(model.detail?.changeBasisPoints, 1000)
        XCTAssertEqual(model.detail?.attribution, "Data provided by CoinGecko")
    }

    func testKeepsTheFailureForRetry() async {
        let model = makeModel(StubTransport.Reply.failure(URLError(.cannotConnectToHost)))

        await model.load()

        if case .failed = model.phase {} else { XCTFail("phase = \(model.phase)") }
        XCTAssertNotNil(model.lastError)
    }

    private func makeModel(_ response: StubTransport.Reply) -> AssetDetailClientModel {
        let api = APIClient(
            serverURL: testServerURL,
            tokens: StubTokenProvider(token: "token-1"),
            transport: StubTransport(scripted: [response])
        )
        return AssetDetailClientModel(api: api, symbol: "AAPLx")
    }

    private let detail = #"""
        {"symbol":"AAPLx","display_name":"Apple xStock","issuer":"xstocks","kind":"equity","logo_url":null,"price_micros":110000000,"price_as_of":null,"change_bps":1000,"sparkline_micros":[],"session":{"state":"open","continuous":false,"holiday":"","early_close":false,"next_state":"after_hours","next_transition":null},"decimals":8,"ui_multiplier":{"num":1,"den":1},"tradable":true,"other_listings":[],"attribution":"Data provided by CoinGecko"}
        """#
}
