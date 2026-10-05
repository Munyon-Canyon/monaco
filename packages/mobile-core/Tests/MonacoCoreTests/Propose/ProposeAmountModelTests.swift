import MonacoAPI
import MonacoCore
import MonacoTestClock
import XCTest

@MainActor
final class ProposeAmountModelTests: XCTestCase {
    func testDebouncesAndCancelsStalePreviews() async {
        let clock = TestClock()
        let service = PreviewService()
        let model = ProposeAmountModel(service: service, cabalID: "cabal", symbol: "GOOGLx", clock: clock)

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
        let model = ProposeAmountModel(service: service, cabalID: "cabal", symbol: "GOOGLx", clock: clock)

        XCTAssertFalse(model.reviewEnabled(assetName: "Alphabet"))
        model.setAmount(micros: 1)
        XCTAssertFalse(model.reviewEnabled(assetName: "Alphabet"))
        await settle()
        clock.advance(by: .milliseconds(400))
        await settle()
        XCTAssertFalse(model.reviewEnabled(assetName: "Alphabet"))
        XCTAssertEqual(model.message(assetName: "Alphabet"), "More than the pot has")
    }

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
