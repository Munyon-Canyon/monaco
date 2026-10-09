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

    func testNoRouteOnASellKeepsTheSmallerAmountLine() {
        XCTAssertEqual(
            ProposePreview(.proposalPreviewNoRoute).message(assetName: "Apple", isSell: true),
            "Can't sell Apple right now. Try a smaller amount.")
    }

    func testAssetPausedBuyTellsTheMemberToTryAnotherStockAndDisablesReview() {
        let preview = ProposePreview(.proposalPreviewAssetPaused)

        XCTAssertEqual(preview.message(assetName: "Starbucks"), "Can't buy Starbucks right now. Try another stock.")
        XCTAssertFalse(preview.reviewEnabled(amountMicros: 2_000_000, isLoading: false, assetName: "Starbucks"))
    }

    func testAssetPausedSellTellsTheMemberToTryAgainLater() {
        let preview = ProposePreview(.proposalPreviewAssetPaused)

        XCTAssertEqual(
            preview.message(assetName: "Starbucks", isSell: true), "Can't sell Starbucks right now. Try again later.")
        XCTAssertFalse(
            preview.reviewEnabled(amountMicros: 2_000_000, isLoading: false, assetName: "Starbucks", isSell: true))
    }

    func testAssetUntradableUsesTheAssetPausedCopy() {
        let preview = ProposePreview(.proposalPreviewAssetUntradable)

        XCTAssertEqual(preview.message(assetName: "Apple"), "Can't buy Apple right now. Try another stock.")
        XCTAssertEqual(
            preview.message(assetName: "Apple", isSell: true), "Can't sell Apple right now. Try again later.")
    }

    private func api(_ transport: StubTransport) -> APIClient {
        APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
    }
}
