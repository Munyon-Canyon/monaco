import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestClock
import XCTest

@MainActor
final class ProposeSellLocationTests: XCTestCase {
    func testASolanaOnlyHoldingKeepsItsUnpinnedSellAndTheApiUnits() {
        let apple = Self.holding(tokenAmount: 120_345_678, valueMicros: 278_470_000, units: "1.2034", listings: nil)

        XCTAssertNil(apple.network)
        XCTAssertEqual(apple.tokenAmount, 120_345_678)
        XCTAssertEqual(apple.locations.map(\.network), [.solana])
        XCTAssertEqual(apple.heldQuantity, "1.2034 shares")
    }

    func testAnEVMOnlyHoldingSellsFromItsListingInTheListingsUnits() {
        let apple = Self.holding(
            tokenAmount: 0, valueMicros: 462_800_000, units: "2.0000",
            listings: [Self.listing(.base, tokenAmount: "200000000", valueMicros: 462_800_000)])

        XCTAssertEqual(apple.network, .base)
        XCTAssertEqual(apple.tokenAmount, 200_000_000)
        XCTAssertEqual(apple.valueMicros, 462_800_000)
        XCTAssertEqual(apple.quantity(of: 100_000_000), "1 share")
        XCTAssertEqual(apple.tokenAmount(forMicros: 231_400_000), 100_000_000)
        XCTAssertEqual(apple.heldQuantity, "2 shares")
    }

    func testAMixedHoldingStartsOnSolanaAndMovesToTheLocationChosen() {
        let apple = Self.holding(
            tokenAmount: 100_000_000, valueMicros: 694_200_000, units: "3.0000",
            listings: [
                Self.listing(.solana, tokenAmount: "100000000", valueMicros: 231_400_000),
                Self.listing(.base, tokenAmount: "200000000", valueMicros: 462_800_000),
            ])
        let onBase = apple.at(.base)

        XCTAssertEqual(apple.network, .solana)
        XCTAssertEqual(apple.tokenAmount, 100_000_000)
        XCTAssertEqual(apple.heldQuantity, "3 shares")
        XCTAssertEqual(onBase.network, .base)
        XCTAssertEqual(onBase.tokenAmount, 200_000_000)
        XCTAssertEqual(onBase.valueMicros, 462_800_000)
        XCTAssertEqual(onBase.helperText, "The cabal holds 2 shares · $462.80")
        XCTAssertEqual(onBase.at(.solana), apple)
        XCTAssertEqual(apple.at(.arbitrum), apple)
    }

    func testAnEmptyListingIsNotAPlaceToSellFrom() async {
        let apple = Self.holding(
            tokenAmount: 100_000_000, valueMicros: 231_400_000, units: "1.0000",
            listings: [Self.listing(.base, tokenAmount: "0", valueMicros: 0)])

        XCTAssertEqual(apple.locations.map(\.network), [.solana])
        XCTAssertNil(apple.network)
    }

    func testAListingPastInt64IsKeptAndCannotBeSoldWhole() async {
        let apple = Self.robinhood25()
        let quarter: Int64 = 1_446_250_000

        XCTAssertEqual(apple.network, .robinhood)
        XCTAssertTrue(apple.overflows)
        XCTAssertEqual(apple.tokenAmount, Int64.max)
        XCTAssertEqual(apple.heldQuantity, "25 shares")
        XCTAssertTrue(apple.exceedsSettleLimit(forMicros: 5_785_000_000))
        XCTAssertTrue(apple.exceedsSettleLimit(forMicros: 2_776_800_000))
        XCTAssertFalse(apple.exceedsSettleLimit(forMicros: 2_082_600_000))
        XCTAssertEqual(apple.tokenAmount(forMicros: 694_200_000), 3_000_000_000_000_000_000)
        XCTAssertEqual(apple.tokenAmount(forMicros: quarter), 6_250_000_000_000_000_000)
        XCTAssertEqual(apple.quantity(of: 3_000_000_000_000_000_000), "3 shares")
    }

    func testASellPastInt64ShowsLowerTheAmountAndNeverReachesReview() async {
        let clock = TestClock()
        let model = ProposeAmountModel(
            service: PreviewService(), cabalID: "cabal", trade: .sell(Self.robinhood25()), clock: clock)

        model.setAmount(micros: 5_785_000_000)
        XCTAssertTrue(model.isOverLimit)
        XCTAssertEqual(
            model.keypadState,
            .invalid(limitLine: ProposeFlowCopy.lowerTheAmount, slot: ProposeFlowCopy.upTo(5_785_000_000)))
        XCTAssertEqual(model.message(assetName: "Apple"), "Lower the amount")
        XCTAssertNil(model.sellQuantityNote)
        XCTAssertFalse(model.reviewEnabled(assetName: "Apple"))

        model.setAmount(micros: 694_200_000)
        XCTAssertFalse(model.isOverLimit)
        XCTAssertEqual(model.sellQuantityNote, "About 3 shares")
        XCTAssertEqual(model.draft, .sell(symbol: "AAPLx", tokenAmount: 3_000_000_000_000_000_000, thesis: ""))
    }

    func testATooLargePreviewSaysLowerTheAmount() async {
        let preview = ProposePreview(
            .init(
                quoteOutAmount: nil, advisoryCode: "trade_too_large",
                advisoryMessage: "This trade is too large to settle. Lower the amount.", potValueMicros: 1))

        XCTAssertEqual(preview.message(assetName: "Apple"), "Lower the amount")
        XCTAssertFalse(preview.reviewEnabled(amountMicros: 1, isLoading: false, assetName: "Apple"))
    }

    func testChoosingALocationClearsTheAmountAndPinsEveryPreviewToIt() async {
        let clock = TestClock()
        let service = PreviewService()
        let apple = Self.mixed()
        let model = ProposeAmountModel(service: service, cabalID: "cabal", trade: .sell(apple), clock: clock)

        model.setAmount(micros: 100_000_000)
        await settle()
        clock.advance(by: .milliseconds(400))
        await settle()
        model.chooseLocation(.base)
        XCTAssertEqual(model.amountMicros, 0)
        XCTAssertNil(model.preview)
        XCTAssertEqual(model.maxMicros, 462_800_000)
        model.setAmount(micros: 462_800_000)
        await settle()
        clock.advance(by: .milliseconds(400))
        await settle()

        let networks = await service.recordedNetworks()
        XCTAssertEqual(networks, [.solana, .base])
        XCTAssertEqual(model.draft, .sell(symbol: "AAPLx", tokenAmount: 200_000_000, thesis: ""))
    }

    func testASellPinNeverFallsBackToAnUnpinnedPreview() async {
        let clock = TestClock()
        let service = PreviewService(pinnedCode: "no_route")
        let model = ProposeAmountModel(service: service, cabalID: "cabal", trade: .sell(Self.mixed()), clock: clock)

        model.setAmount(micros: 100_000_000)
        await settle()
        clock.advance(by: .milliseconds(400))
        await settle()

        let networks = await service.recordedNetworks()
        XCTAssertEqual(networks, [.solana])
        XCTAssertEqual(model.message(assetName: "Apple"), "Can't sell Apple right now. Try a smaller amount.")
    }

    func testReviewNamesTheChainAndSendsTheProposalPinnedToIt() async {
        let service = ReviewService()
        let onBase = Self.mixed().at(.base)
        let model = ProposeReviewModel(
            service: service, cabalID: "cabal", cabal: .init(name: "Sunday Investors", voters: "All 3 members"),
            draft: .sell(symbol: "AAPLx", tokenAmount: 100_000_000, thesis: "Trim"),
            preview: ProposePreview(
                .init(quoteOutAmount: 231_000_000, advisoryCode: nil, advisoryMessage: nil, potValueMicros: 1)),
            trade: .sell(onBase))

        XCTAssertEqual(model.rows.first, .init(label: "Sells on", value: "Base"))
        XCTAssertEqual(model.rows.last, .init(label: "Who votes", value: "All 3 members"))
        XCTAssertEqual(model.rows.first { $0.label == "Cabal keeps" }?.value, "1 share")
        await model.send()
        let sent = await service.networks
        XCTAssertEqual(sent, [.base])
    }

    func testASolanaOnlySellKeepsSendingNoNetwork() async {
        let service = ReviewService()
        let apple = Self.holding(tokenAmount: 120_345_678, valueMicros: 278_470_000, units: "1.2034", listings: nil)
        let model = ProposeReviewModel(
            service: service, cabalID: "cabal", cabal: .init(name: "Sunday Investors", voters: "All 3 members"),
            draft: .sell(symbol: "AAPLx", tokenAmount: 60_172_839, thesis: "Trim"),
            preview: ProposePreview(
                .init(quoteOutAmount: 139_000_000, advisoryCode: nil, advisoryMessage: nil, potValueMicros: 1)),
            trade: .sell(apple))

        XCTAssertNil(model.rows.first { $0.label == "Sells on" })
        await model.send()
        let sent = await service.networks
        XCTAssertEqual(sent, [nil])
    }

    private func settle() async {
        for _ in 0..<10 { await Task.yield() }
    }

    static func robinhood25() -> ProposeHolding {
        holding(
            tokenAmount: 0, valueMicros: 5_785_000_000, units: "25.0000",
            listings: [listing(.robinhood, tokenAmount: "25000000000000000000", valueMicros: 5_785_000_000)])
    }

    static func mixed() -> ProposeHolding {
        holding(
            tokenAmount: 100_000_000, valueMicros: 694_200_000, units: "3.0000",
            listings: [
                listing(.solana, tokenAmount: "100000000", valueMicros: 231_400_000),
                listing(.base, tokenAmount: "200000000", valueMicros: 462_800_000),
            ])
    }

    static func listing(
        _ network: Components.Schemas.Network, tokenAmount: String, valueMicros: Int64
    ) -> Components.Schemas.HoldingListing {
        .init(
            listingId: UUID().uuidString.lowercased(), issuer: "coinbase", network: network, tokenAmount: tokenAmount,
            valueMicros: valueMicros)
    }

    static func holding(
        tokenAmount: Int64, valueMicros: Int64, units: String, listings: [Components.Schemas.HoldingListing]?
    ) -> ProposeHolding {
        ProposeHolding(
            Components.Schemas.CabalHolding(
                listings: listings, symbol: "AAPLx", displayName: "Apple", kind: .equity, units: units,
                tokenAmount: tokenAmount, priceMicros: 231_400_000, valueMicros: valueMicros, weightBps: 5000,
                costBasisMicros: valueMicros, pnlMicros: 0))
    }
}
