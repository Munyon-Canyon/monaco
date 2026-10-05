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

    private func makeModel(kind: AssetKind, service: ReviewService, thesis: String = "Earnings next week.")
        -> ProposeReviewModel
    {
        let preview = ProposePreview(
            .init(quoteOutAmount: 73_000_000, advisoryCode: nil, advisoryMessage: nil, potValueMicros: 500_000_000))
        return ProposeReviewModel(
            service: service, cabalID: "cabal", cabal: .init(name: "Sunday Investors", voters: "All 3 members"),
            draft: .buy(symbol: "GOOGL", usdcMicros: 250_000_000, thesis: thesis), preview: preview, kind: kind,
            tokenDecimals: 8)
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
