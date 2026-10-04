import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

final class ProposePreviewTests: XCTestCase {
    func testPreviewDecodesTheWireValues() async throws {
        let transport = StubTransport(
            .json(
                .ok,
                #"{"quote_out_amount":21000000,"advisory_code":null,"advisory_message":null,"pot_value_micros":100000000}"#
            )
        )
        let preview = try await LiveProposeService(api: api(transport)).preview(
            cabalID: "cabal", draft: .buy(symbol: "AAPLx", usdcMicros: 5_000_000, thesis: "")
        )

        XCTAssertEqual(preview.quoteOutAmount, 21_000_000)
        XCTAssertNil(preview.advisoryCode)
        XCTAssertEqual(preview.maxMicros, 100_000_000)
        XCTAssertEqual(preview.potHelperText, "The pot has $100.00")
    }

    func testPotExceededDisablesReview() {
        let preview = ProposePreview(.proposalPreviewPotExceeded)

        XCTAssertEqual(preview.message(assetName: "Apple"), "More than the pot has")
        XCTAssertFalse(preview.reviewEnabled(amountMicros: 1, isLoading: false, assetName: "Apple"))
    }

    func testNoRouteDisablesReviewAndUsesStockName() {
        let preview = ProposePreview(.proposalPreviewNoRoute)

        XCTAssertEqual(
            preview.message(assetName: "Apple"), "Can't buy Apple right now. Try a smaller amount or another stock.")
        XCTAssertFalse(preview.reviewEnabled(amountMicros: 1, isLoading: false, assetName: "Apple"))
        XCTAssertFalse(
            ProposePreview(.proposalPreviewClean).reviewEnabled(amountMicros: 0, isLoading: false, assetName: "Apple"))
        XCTAssertFalse(
            ProposePreview(.proposalPreviewClean).reviewEnabled(amountMicros: 1, isLoading: true, assetName: "Apple"))
    }

    func testAssetUntradableUsesTheSameInlineCopy() {
        XCTAssertEqual(
            ProposePreview(.proposalPreviewAssetUntradable).message(assetName: "Apple"),
            "Can't buy Apple right now. Try a smaller amount or another stock."
        )
    }

    private func api(_ transport: StubTransport) -> APIClient {
        APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
    }
}
