import Foundation
import MonacoAPI
import XCTest

@testable import MonacoCore

@MainActor
final class ProposeReviewThesisTests: XCTestCase {
    func testSetThesisKeepsAtMost280Characters() {
        let model = makeModel(kind: .stock, service: ReviewService(), thesis: "")

        model.setThesis(String(repeating: "a", count: 300))

        XCTAssertEqual(model.thesis.count, 280)
    }

    func testTheCounterAppearsFrom180Characters() {
        let model = makeModel(kind: .stock, service: ReviewService(), thesis: "")

        model.setThesis(String(repeating: "a", count: 179))
        XCTAssertNil(model.thesisCounter)
        model.setThesis(String(repeating: "a", count: 197))
        XCTAssertEqual(model.thesisCounter, "197 / 280")
    }

    func testASeededThesisStartsTheModelAndReadsAsTheReason() {
        let model = makeModel(thesis: "Earnings next week.")

        XCTAssertEqual(model.thesis, "Earnings next week.")
        XCTAssertEqual(model.reason, "Earnings next week.")
    }

    func testTheReasonFollowsWhatWasTyped() {
        let model = makeModel(thesis: "Earnings next week.")

        model.setThesis("  Cloud is growing.  ")
        XCTAssertEqual(model.reason, "Cloud is growing.")
        model.setThesis("   ")
        XCTAssertNil(model.reason)
    }

    func testSendingABuySendsTheTypedThesis() async {
        let service = ReviewService()
        let model = makeModel(kind: .stock, service: service, thesis: "")

        model.setThesis("Cloud is growing.")
        await model.send()

        let drafts = await service.drafts
        XCTAssertEqual(drafts, [.buy(symbol: "GOOGL", usdcMicros: 250_000_000, thesis: "Cloud is growing.")])
    }

    func testSendingAPreIpoBuySendsTheTypedThesis() async {
        let service = ReviewService()
        let model = makeModel(kind: .preIpo, service: service, thesis: "")

        model.setThesis("Late-stage round soon.")
        await model.send()

        let drafts = await service.drafts
        XCTAssertEqual(drafts.first?.thesis, "Late-stage round soon.")
    }

    func testSendingASellSendsTheTypedThesis() async {
        let apple = ProposeHoldingTests.holding(name: "Apple", units: "1.2034", tokenAmount: 120_345_678)
        let service = ReviewService()
        let preview = ProposePreview(
            .init(quoteOutAmount: 139_000_000, advisoryCode: nil, advisoryMessage: nil, potValueMicros: 500_000_000))
        let model = ProposeReviewModel(
            service: service, cabalID: "cabal", cabal: .init(name: "Sunday Investors", voters: "All 3 members"),
            draft: .sell(symbol: apple.symbol, tokenAmount: 60_172_839, thesis: ""), preview: preview,
            trade: .sell(apple))

        model.setThesis("Take profit.")
        await model.send()

        let drafts = await service.drafts
        XCTAssertEqual(drafts, [.sell(symbol: apple.symbol, tokenAmount: 60_172_839, thesis: "Take profit.")])
    }

    func testAnEmptyOrBlankBoxSendsAnEmptyThesisAndStaysSendable() async {
        let service = ReviewService()
        let model = makeModel(kind: .stock, service: service, thesis: "")

        XCTAssertTrue(model.canSend)
        model.setThesis("   ")
        XCTAssertTrue(model.canSend)
        await model.send()

        let drafts = await service.drafts
        XCTAssertEqual(drafts.first?.thesis.trimmingCharacters(in: .whitespacesAndNewlines), "")
        XCTAssertEqual(model.proposalID, "proposal-1")
    }

    func testTheMemoryKeepsWhatWasTypedAcrossARepresent() {
        let memory = ProposeReviewMemory()
        let first = memory.hold(makeModel(thesis: "Earnings next week."))
        first.setThesis("Changed my mind.")

        let again = memory.model(for: .buy(symbol: "GOOGL", usdcMicros: 250_000_000, thesis: "Earnings next week."))

        XCTAssertTrue(again === first)
        XCTAssertEqual(again?.thesis, "Changed my mind.")
    }

    private func makeModel(
        kind: AssetKind = .stock, service: ReviewService = ReviewService(), thesis: String
    ) -> ProposeReviewModel {
        let preview = ProposePreview(
            .init(quoteOutAmount: 73_000_000, advisoryCode: nil, advisoryMessage: nil, potValueMicros: 500_000_000))
        return ProposeReviewModel(
            service: service, cabalID: "cabal", cabal: .init(name: "Sunday Investors", voters: "All 3 members"),
            draft: .buy(symbol: "GOOGL", usdcMicros: 250_000_000, thesis: thesis), preview: preview,
            trade: .buy(symbol: "GOOGL", kind: kind, tokenDecimals: 8), buyName: "Alphabet")
    }
}
