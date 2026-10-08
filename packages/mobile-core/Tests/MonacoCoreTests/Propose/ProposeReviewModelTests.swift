import MonacoAPI
import MonacoCore
import XCTest

@MainActor
final class ProposeReviewModelTests: XCTestCase {
    func testBuyRowsSayShares() {
        let model = makeModel(kind: .stock, service: ReviewService())

        XCTAssertEqual(model.title, "Buy $250.00 of GOOGL")
        XCTAssertEqual(
            model.rows,
            [
                .init(label: "Cabal gets", value: "about 0.73 shares"),
                .init(label: "Price", value: "about $342.47 a share"),
                .init(label: "Pot", value: "50% of $500.00"),
                .init(label: "Who votes", value: "All 3 members"),
            ])
        XCTAssertEqual(model.reason, "Earnings next week.")
    }

    func testPreIpoRowsSayTokens() {
        let model = makeModel(kind: .preIpo, service: ReviewService(), thesis: "  ")

        XCTAssertEqual(model.rows.first?.value, "about 0.73 tokens")
        XCTAssertEqual(model.rows[1].value, "about $342.47 a token")
        XCTAssertNil(model.reason)
    }

    func testNineDecimalPreIpoBuyCountsOneToken() {
        let model = makeModel(kind: .preIpo, service: ReviewService(), quoteOut: 1_000_000_000, tokenDecimals: 9)

        XCTAssertEqual(model.rows[0], .init(label: "Cabal gets", value: "about 1 token"))
        XCTAssertEqual(model.rows[1], .init(label: "Price", value: "about $250.00 a token"))
    }

    func testUnknownDecimalsLeaveOutCountAndPrice() {
        let model = makeModel(kind: .preIpo, service: ReviewService(), tokenDecimals: nil)

        XCTAssertEqual(
            model.rows,
            [
                .init(label: "Pot", value: "50% of $500.00"),
                .init(label: "Who votes", value: "All 3 members"),
            ])
    }

    func testSendReturnsTheProposalAndNamesTheCabal() async {
        let service = ReviewService()
        let model = makeModel(kind: .stock, service: service)

        XCTAssertEqual(model.sendTitle, "Send to cabal")
        await model.send()

        XCTAssertEqual(model.proposalID, "proposal-1")
        XCTAssertEqual(model.successToast, "Proposal sent to Sunday Investors")
    }

    func testRetryReplaysTheSameSubmission() async {
        let service = ReviewService(failures: 1)
        let model = makeModel(kind: .stock, service: service)

        await model.send()
        XCTAssertNotNil(model.errorMessage)
        XCTAssertNil(model.proposalID)
        await model.send()

        let submissions = await service.submissions
        XCTAssertEqual(submissions.count, 2)
        XCTAssertTrue(submissions[0] === submissions[1])
        XCTAssertNil(model.errorMessage)
        XCTAssertEqual(model.proposalID, "proposal-1")
    }

    func testSellSaysSharesRaisesAndWhatTheCabalKeeps() {
        let apple = ProposeHoldingTests.holding(name: "Apple", units: "1.2034", tokenAmount: 120_345_678)
        let model = makeSellModel(apple, selling: 60_172_839)

        XCTAssertEqual(model.title, "Sell 0.6017 shares of AAPL")
        XCTAssertEqual(
            model.rows,
            [
                .init(label: "Raises", value: "about $139.00"),
                .init(label: "Cabal keeps", value: "0.6017 shares"),
                .init(label: "Who votes", value: "All 3 members"),
            ])
        XCTAssertEqual(model.reasonTitle, "Why sell")
    }

    func testPreIpoSellSaysTokens() {
        let spacex = ProposeHoldingTests.holding(
            name: "SpaceX", kind: .preIpo, units: "2.0000", tokenAmount: 2_000_000_000)
        let model = makeSellModel(spacex, selling: 2_000_000_000)

        XCTAssertEqual(model.title, "Sell 2 tokens of SPACEX")
        XCTAssertEqual(model.rows[1], .init(label: "Cabal keeps", value: "0 tokens"))
    }

    private func makeSellModel(_ holding: ProposeHolding, selling amount: Int64) -> ProposeReviewModel {
        let preview = ProposePreview(
            .init(quoteOutAmount: 139_000_000, advisoryCode: nil, advisoryMessage: nil, potValueMicros: 500_000_000))
        return ProposeReviewModel(
            service: ReviewService(), cabalID: "cabal", cabal: .init(name: "Sunday Investors", voters: "All 3 members"),
            draft: .sell(symbol: holding.symbol, tokenAmount: amount, thesis: ""), preview: preview,
            trade: .sell(holding))
    }

    private func makeModel(
        kind: AssetKind, service: ReviewService, thesis: String = "Earnings next week.",
        quoteOut: Int64 = 73_000_000, tokenDecimals: Int? = 8
    ) -> ProposeReviewModel {
        let preview = ProposePreview(
            .init(quoteOutAmount: quoteOut, advisoryCode: nil, advisoryMessage: nil, potValueMicros: 500_000_000))
        return ProposeReviewModel(
            service: service, cabalID: "cabal", cabal: .init(name: "Sunday Investors", voters: "All 3 members"),
            draft: .buy(symbol: "GOOGL", usdcMicros: 250_000_000, thesis: thesis), preview: preview,
            trade: .buy(symbol: "GOOGL", kind: kind, tokenDecimals: tokenDecimals))
    }
}

private actor ReviewService: ProposeService {
    struct Failure: Error {}
    private var failures: Int
    private(set) var submissions: [IdempotentSubmission] = []

    init(failures: Int = 0) { self.failures = failures }

    func preview(cabalID _: String, draft _: ProposalDraft) async throws -> ProposePreview { throw Failure() }

    func propose(cabalID _: String, draft _: ProposalDraft, submission: IdempotentSubmission) async throws -> String {
        submissions.append(submission)
        if failures > 0 {
            failures -= 1
            throw Failure()
        }
        return "proposal-1"
    }

    func withdraw(proposalID _: String, submission _: IdempotentSubmission) async throws {}
}
