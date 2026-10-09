import MonacoAPI
import MonacoCore
import MonacoTestClock
import XCTest

@MainActor
final class ProposeAmountModelTests: XCTestCase {
    func testDebouncesAndCancelsStalePreviews() async {
        let clock = TestClock()
        let service = PreviewService()
        let model = ProposeAmountModel(service: service, cabalID: "cabal", trade: Self.buy, clock: clock)

        model.setAmount(micros: 25_000_000)
        await settle()
        model.setAmount(micros: 50_000_000)
        await settle()
        clock.advance(by: .milliseconds(400))
        await settle()

        let amounts = await service.recordedAmounts()
        XCTAssertEqual(amounts, [50_000_000])
        XCTAssertEqual(model.maxMicros, 500_000_000)
    }

    func testTypingAReasonKeepsItAndStopsAtTheLimit() {
        let model = ProposeAmountModel(service: PreviewService(), cabalID: "cabal", trade: Self.buy, clock: TestClock())

        model.setThesis("Earnings next week")
        XCTAssertEqual(model.thesis, "Earnings next week")
        model.setThesis(String(repeating: "a", count: ProposeReasonRules.thesisLimit + 20))

        XCTAssertEqual(model.thesis.count, ProposeReasonRules.thesisLimit)
    }

    func testReviewGatesZeroLoadingAndAdvisories() async {
        let clock = TestClock()
        let service = PreviewService(code: "pot_exceeded")
        let model = ProposeAmountModel(service: service, cabalID: "cabal", trade: Self.buy, clock: clock)

        XCTAssertFalse(model.reviewEnabled(assetName: "Alphabet"))
        model.setAmount(micros: 1)
        XCTAssertFalse(model.reviewEnabled(assetName: "Alphabet"))
        await settle()
        clock.advance(by: .milliseconds(400))
        await settle()
        XCTAssertFalse(model.reviewEnabled(assetName: "Alphabet"))
        XCTAssertEqual(model.message(assetName: "Alphabet"), "More than the pot has")
    }

    func testSellSendsTokensAndStopsAboveTheHolding() async {
        let clock = TestClock()
        let service = PreviewService()
        let apple = ProposeHoldingTests.holding(
            name: "Apple", units: "1.2034", tokenAmount: 120_345_678, valueMicros: 278_470_000)
        let model = ProposeAmountModel(service: service, cabalID: "cabal", trade: .sell(apple), clock: clock)

        XCTAssertEqual(model.maxMicros, 278_470_000)
        XCTAssertEqual(model.helperText, "The cabal holds 1.2034 shares · $278.47")
        XCTAssertNil(model.sellQuantityNote)
        XCTAssertEqual(model.reviewTitle, "Review")
        XCTAssertEqual(model.overLimitHelper, "More than the cabal holds")
        model.setAmount(micros: 278_470_000)
        await settle()
        clock.advance(by: .milliseconds(400))
        await settle()
        XCTAssertEqual(model.draft, .sell(symbol: "AAPLx", tokenAmount: 120_345_678, thesis: ""))
        XCTAssertTrue(model.reviewEnabled(assetName: "Apple"))

        model.setAmount(micros: 278_480_000)
        await settle()
        clock.advance(by: .milliseconds(400))
        await settle()
        XCTAssertTrue(model.isOverLimit)
        XCTAssertFalse(model.reviewEnabled(assetName: "Apple"))
        let amounts = await service.recordedAmounts()
        XCTAssertEqual(amounts.first, 120_345_678)
    }

    func testSellShowsTheShareCountBeforeReview() {
        let apple = ProposeHoldingTests.holding(
            name: "Apple", units: "1.0000", tokenAmount: 100_000_000, valueMicros: 100_000_000)
        let model = ProposeAmountModel(
            service: PreviewService(), cabalID: "cabal", trade: .sell(apple), clock: TestClock())

        model.setAmount(micros: 25_000_000)

        XCTAssertEqual(model.sellQuantityNote, "About 0.25 shares")
        XCTAssertEqual(model.reviewTitle, "Review $25.00")
    }

    func testUnpricedHoldingSaysSoAndCannotBeReviewed() async {
        let clock = TestClock()
        let apple = ProposeHoldingTests.holding(name: "Apple", valueMicros: 0)
        let model = ProposeAmountModel(
            service: PreviewService(), cabalID: "cabal", trade: .sell(apple), clock: clock)

        XCTAssertEqual(model.helperText, "No price for this stock right now")
        model.setAmount(micros: 1_000_000)
        await settle()
        clock.advance(by: .milliseconds(400))
        await settle()
        XCTAssertEqual(model.overLimitHelper, "No price for this stock right now")
        XCTAssertNil(model.sellQuantityNote)
        XCTAssertFalse(model.reviewEnabled(assetName: "Apple"))
    }

    func testBuyOverTheLimitSaysThePot() {
        let model = ProposeAmountModel(
            service: PreviewService(), cabalID: "cabal", trade: Self.buy, clock: TestClock())

        XCTAssertEqual(model.overLimitHelper, "More than the pot has")
    }

    func testResolveAssetFixesABuyAndLeavesASellAlone() {
        let buy = ProposeAmountModel(
            service: PreviewService(), cabalID: "cabal",
            trade: .buy(symbol: "SPACEX", kind: .stock, tokenDecimals: nil),
            clock: TestClock())
        buy.resolveAsset(kind: .preIpo, decimals: 9)
        XCTAssertEqual(buy.trade, .buy(symbol: "SPACEX", kind: .preIpo, tokenDecimals: 9))

        let apple = ProposeHoldingTests.holding(name: "Apple", units: "1.2034", tokenAmount: 120_345_678)
        let sell = ProposeAmountModel(
            service: PreviewService(), cabalID: "cabal", trade: .sell(apple), clock: TestClock())
        sell.resolveAsset(kind: .preIpo, decimals: 9)
        XCTAssertEqual(sell.trade, .sell(apple))
    }

    func testMaxAndHelperAreAvailableBeforeTyping() async {
        let model = ProposeAmountModel(service: PreviewService(), cabalID: "cabal", trade: Self.buy, clock: TestClock())
        XCTAssertNil(model.maxMicros)
        XCTAssertNil(model.helperText)

        await model.load()

        XCTAssertEqual(model.maxMicros, 500_000_000)
        XCTAssertEqual(model.helperText, "The pot has $500.00")
    }

    func testHelperKeepsItsTextWhileANewPreviewLoads() async {
        let clock = TestClock()
        let service = PreviewService()
        let model = ProposeAmountModel(service: service, cabalID: "cabal", trade: Self.buy, clock: clock)
        await model.load()

        model.setAmount(micros: 25_000_000)

        XCTAssertTrue(model.isLoading)
        XCTAssertEqual(model.helperText, "The pot has $500.00")
        XCTAssertEqual(model.maxMicros, 500_000_000)
    }

    func testReviewStaysDisabledWhileThePreviewIsForAnOlderAmount() async {
        let clock = TestClock()
        let service = PreviewService()
        let model = ProposeAmountModel(service: service, cabalID: "cabal", trade: Self.buy, clock: clock)
        model.setAmount(micros: 25_000_000)
        await settle()
        clock.advance(by: .milliseconds(400))
        await settle()
        XCTAssertTrue(model.reviewEnabled(assetName: "Alphabet"))

        await service.hold()
        model.setAmount(micros: 50_000_000)
        await settle()
        clock.advance(by: .milliseconds(400))
        await settle()

        XCTAssertNotNil(model.preview)
        XCTAssertEqual(model.previewAmountMicros, 25_000_000)
        XCTAssertFalse(model.reviewEnabled(assetName: "Alphabet"))
        await service.release()
        await settle()
        XCTAssertEqual(model.previewAmountMicros, 50_000_000)
        XCTAssertTrue(model.reviewEnabled(assetName: "Alphabet"))
    }

    func testAFailedPreviewIsFlaggedAndRetryRecovers() async {
        let clock = TestClock()
        let service = PreviewService()
        let model = ProposeAmountModel(service: service, cabalID: "cabal", trade: Self.buy, clock: clock)
        await service.setFailing(true)

        model.setAmount(micros: 30_000_000)
        await settle()
        clock.advance(by: .milliseconds(400))
        await settle()

        XCTAssertTrue(model.previewFailed)
        XCTAssertFalse(model.isLoading)
        XCTAssertFalse(model.reviewEnabled(assetName: "Alphabet"))

        await service.setFailing(false)
        model.retryPreview()
        XCTAssertFalse(model.previewFailed)
        await settle()
        clock.advance(by: .milliseconds(400))
        await settle()

        XCTAssertFalse(model.previewFailed)
        XCTAssertTrue(model.reviewEnabled(assetName: "Alphabet"))
    }

    private static let buy = ProposeTrade.buy(symbol: "GOOGLx", kind: .stock, tokenDecimals: 8)

    private func settle() async {
        for _ in 0..<10 { await Task.yield() }
    }
}

private actor PreviewService: ProposeService {
    struct Failure: Error {}
    let code: String?
    var amounts: [Int64] = []
    private var failing = false
    private var holding = false
    private var held: [CheckedContinuation<Void, Never>] = []

    init(code: String? = nil) { self.code = code }

    func setFailing(_ value: Bool) { failing = value }
    func hold() { holding = true }
    func release() {
        holding = false
        for continuation in held { continuation.resume() }
        held = []
    }

    func potValue(cabalID _: String) async throws -> Int64 { 500_000_000 }

    func preview(cabalID _: String, draft: ProposalDraft) async throws -> ProposePreview {
        amounts.append(draft.amount)
        if failing { throw Failure() }
        if holding { await withCheckedContinuation { held.append($0) } }
        return ProposePreview(
            .init(quoteOutAmount: nil, advisoryCode: code, advisoryMessage: nil, potValueMicros: 500_000_000))
    }

    func propose(cabalID _: String, draft _: ProposalDraft, submission _: IdempotentSubmission) async throws -> String {
        "proposal"
    }
    func withdraw(proposalID _: String, submission _: IdempotentSubmission) async throws {}
    func recordedAmounts() -> [Int64] { amounts }
}
