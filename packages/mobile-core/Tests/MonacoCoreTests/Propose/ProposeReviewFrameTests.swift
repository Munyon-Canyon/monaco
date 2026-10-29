import Foundation
import MonacoAPI
import XCTest

@testable import MonacoCore

@MainActor
final class ProposeReviewFrameTests: XCTestCase {
    private static let ruled = ProposeCabalInfo(
        name: "AC Capital", voters: "All 8 members", votingWindowShort: "6 h", voterCount: 8, neededToPass: 5)

    private func spaceXProfile() throws -> PreIpoDetail.Profile {
        try XCTUnwrap(PreIpoDetail(.spaceX, locale: Locale(identifier: "en_US"))?.profile)
    }

    private func frameModel(kind: AssetKind = .stock, amount: Int64 = 400_000_000) -> ProposeReviewModel {
        let preview = ProposePreview(
            .init(
                quoteOutAmount: 182_000_000, advisoryCode: nil, advisoryMessage: nil,
                potValueMicros: 20_000_000_000, spendableCashMicros: 5_100_000_000))
        return ProposeReviewModel(
            service: ReviewService(), cabalID: "cabal", cabal: Self.ruled,
            draft: .buy(symbol: "NVDA", usdcMicros: amount, thesis: ""), preview: preview,
            trade: .buy(symbol: "NVDA", kind: kind, tokenDecimals: 8), buyName: "NVIDIA")
    }

    func testListedBuyLinesAndStatsReadAsTheFrame() {
        let model = frameModel()

        XCTAssertEqual(model.assetLine, "Buy · market order")
        XCTAssertEqual(model.amountLabel, "$400")
        XCTAssertEqual(model.cabalLine, "Cash $5,100 · 6 h to vote")
        XCTAssertTrue(model.showsThesis)
        XCTAssertEqual(
            model.stats.map { "\($0.label) \($0.value)" },
            ["Shares ≈ 1.82", "Price $219.78", "Of treasury 2%", "To pass 5 of 8"])
        XCTAssertEqual(model.preIpoLedger, [])
        XCTAssertEqual(model.sendTitle, "Send to vote")
    }

    func testSellLineSaysSellAndStatsUseTheExactShares() {
        let apple = ProposeHoldingTests.holding(name: "Apple", units: "1.2034", tokenAmount: 120_345_678)
        let model = makeSellModel(apple, selling: 60_172_839)

        XCTAssertEqual(model.assetLine, "Sell · market order")
        XCTAssertEqual(model.amountLabel, "0.6017 shares")
        XCTAssertEqual(model.stats[0].value, "0.6017")
        XCTAssertEqual(model.stats[1].value, UsdAmountFormatter.format(micros: apple.priceMicros))
        XCTAssertEqual(model.stats[2].value, "28%")
        XCTAssertEqual(model.stats[3].value, ProposalStats.missing)
        XCTAssertEqual(model.cabalLine, "")
    }

    func testPreIpoBuyUsesTheEstimateLedgerAndPutsToPassOnTheCabalRow() async throws {
        let model = frameModel(kind: .preIpo, amount: 500_000_000)
        XCTAssertEqual(model.assetLine, "Buy at the estimate")
        XCTAssertEqual(model.preIpoLedger, [])
        let bare = ProposeCabalInfo(name: "A", voters: "B")
        let empty = ProposePreview(
            .init(quoteOutAmount: nil, advisoryCode: nil, advisoryMessage: nil, potValueMicros: 0))
        let draft = ProposalDraft.buy(symbol: "S", usdcMicros: 1, thesis: "")
        let bareModel = ProposeReviewModel(
            service: ReviewService(), cabalID: "c", cabal: bare, draft: draft, preview: empty,
            trade: .buy(symbol: "S", kind: .preIpo, tokenDecimals: 8))
        XCTAssertEqual(bareModel.cabalLine, "— to pass")
        model.setPreIpo(try spaceXProfile())

        XCTAssertEqual(model.assetLine, "Buy ≈ 2.70 sh at the estimate")
        XCTAssertEqual(model.cabalLine, "Cash $5,100 · 6 h · 5 of 8 to pass")
        XCTAssertFalse(model.showsThesis)
        XCTAssertEqual(
            model.preIpoLedger,
            [
                PreIpoCopy.LedgerLine(label: "Last round · Jun ’26", value: "$165.00"),
                PreIpoCopy.LedgerLine(label: "Secondary estimate · Oct 20", value: "$185.00"),
            ])
    }

    func testMissingQuoteAndPotShowDashes() {
        let preview = ProposePreview(
            .init(quoteOutAmount: nil, advisoryCode: nil, advisoryMessage: nil, potValueMicros: 0))
        let model = ProposeReviewModel(
            service: ReviewService(), cabalID: "cabal", cabal: .init(name: "A", voters: "B"),
            draft: .buy(symbol: "NVDA", usdcMicros: 1, thesis: ""), preview: preview,
            trade: .buy(symbol: "NVDA", kind: .stock, tokenDecimals: 8))

        XCTAssertEqual(model.stats.map(\.value), ["—", "—", "—", "—"])
        XCTAssertEqual(model.cabalLine, "")
    }

    func testCabalInfoCountsVotersAndWhatPasses() {
        var cabal = Components.Schemas.Cabal.sampleWithMembers(role: "member")
        cabal.rules.threshold = "majority"
        XCTAssertEqual(ProposeCabalInfo(cabal).voterCount, 3)
        XCTAssertEqual(ProposeCabalInfo(cabal).neededToPass, 2)
        cabal.rules.threshold = "unanimous"
        XCTAssertEqual(ProposeCabalInfo(cabal).neededToPass, 3)
        XCTAssertEqual(ProposeCabalInfo.shortWindow(seconds: 21_600), "6 h")
        XCTAssertEqual(ProposeCabalInfo.shortWindow(seconds: 604_800), "7 d")
        XCTAssertEqual(ProposeCabalInfo.shortWindow(seconds: 60), "1 h")
    }

    private func makeSellModel(_ holding: ProposeHolding, selling amount: Int64) -> ProposeReviewModel {
        let preview = ProposePreview(
            .init(quoteOutAmount: 139_000_000, advisoryCode: nil, advisoryMessage: nil, potValueMicros: 500_000_000))
        return ProposeReviewModel(
            service: ReviewService(), cabalID: "cabal", cabal: .init(name: "Sunday Investors", voters: "All 3 members"),
            draft: .sell(symbol: holding.symbol, tokenAmount: amount, thesis: ""), preview: preview,
            trade: .sell(holding))
    }
}
