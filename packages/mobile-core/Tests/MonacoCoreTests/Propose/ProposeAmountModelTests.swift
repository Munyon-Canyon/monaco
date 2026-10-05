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
        XCTAssertEqual(model.helperText, "The cabal holds $278.47")
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

    private static let buy = ProposeTrade.buy(symbol: "GOOGLx", kind: .stock, tokenDecimals: 8)

    private func settle() async {
        for _ in 0..<10 { await Task.yield() }
    }
}

private actor PreviewService: ProposeService {
    let code: String?
    var amounts: [Int64] = []

    init(code: String? = nil) { self.code = code }

    func preview(cabalID _: String, draft: ProposalDraft) async throws -> ProposePreview {
        amounts.append(draft.amount)
        return ProposePreview(
            .init(quoteOutAmount: nil, advisoryCode: code, advisoryMessage: nil, potValueMicros: 500_000_000))
    }

    func propose(cabalID _: String, draft _: ProposalDraft, submission _: IdempotentSubmission) async throws -> String {
        "proposal"
    }
    func withdraw(proposalID _: String, submission _: IdempotentSubmission) async throws {}
    func recordedAmounts() -> [Int64] { amounts }
}
