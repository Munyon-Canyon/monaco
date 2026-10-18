import Foundation
import MonacoAPI
import XCTest

@testable import MonacoCore

final class ProposalBestPriceTests: XCTestCase {
    func testAProposalCarriesItsComparisonForItsSide() {
        var buy = Components.Schemas.Proposal.sample(kind: .buy)
        buy.priceComparison = BestPrice.sampleComparison(now: Date())
        var sell = Components.Schemas.Proposal.sample(kind: .sell)
        sell.priceComparison = BestPrice.sampleComparison(now: Date())

        XCTAssertEqual(ProposalSummary(buy).bestPrice?.isSell, false)
        XCTAssertEqual(ProposalSummary(buy).bestPrice?.venues.count, 3)
        XCTAssertEqual(ProposalSummary(sell).bestPrice?.isSell, true)
        XCTAssertNil(ProposalSummary(.sample()).bestPrice)
    }

    func testAProposalDetailCarriesItsComparison() {
        var detail = Components.Schemas.ProposalDetail.sample(kind: .sell)
        detail.priceComparison = BestPrice.sampleComparison(now: Date())

        let summary = ProposalDetail(detail).summary

        XCTAssertEqual(summary.bestPrice?.venues.count, 3)
        XCTAssertEqual(summary.bestPrice?.isSell, true)
        XCTAssertNil(ProposalDetail(.sample()).summary.bestPrice)
    }

    func testAVoteKeepsTheComparison() {
        var proposal = Components.Schemas.Proposal.sample()
        proposal.priceComparison = BestPrice.sampleComparison(now: Date())
        let summary = ProposalSummary(proposal)

        let voted = ProposalSummary(summary, ballot: "yes", tally: summary.tally)

        XCTAssertEqual(voted.bestPrice, summary.bestPrice)
        XCTAssertEqual(ProposalSummary(retrying: summary).bestPrice, summary.bestPrice)
    }

    func testAProposalDetailCarriesItsPinnedChainThroughAVote() {
        var detail = Components.Schemas.ProposalDetail.sample()
        detail.network = .base

        let summary = ProposalDetail(detail).summary
        let voted = ProposalSummary(summary, ballot: "yes", tally: summary.tally)

        XCTAssertEqual(summary.network, .base)
        XCTAssertEqual(voted.network, .base)
        XCTAssertEqual(ProposalSummary(retrying: summary).network, .base)
        XCTAssertNil(ProposalDetail(.sample()).summary.network)
    }

    func testChainNameNamesEveryChain() {
        XCTAssertEqual(BestPrice.chainName(.robinhood), "Robinhood Chain")
        XCTAssertEqual(BestPrice.chainName(.base), "Base")
    }

    func testTheReviewShowsThePreviewsComparison() async {
        let preview = ProposePreview(
            .init(
                priceComparison: BestPrice.sampleComparison(now: Date()), quoteOutAmount: 73_000_000, advisoryCode: nil,
                advisoryMessage: nil, potValueMicros: 500_000_000))
        let bestPrice = await MainActor.run {
            ProposeReviewModel(
                service: UnusedProposeService(), cabalID: "cabal",
                cabal: .init(name: "Sunday Investors", voters: "All 3 members"),
                draft: .buy(symbol: "GOOGL", usdcMicros: 10_000_000, thesis: ""), preview: preview,
                trade: .buy(symbol: "GOOGL", kind: .stock, tokenDecimals: 8)
            ).bestPrice
        }

        XCTAssertEqual(bestPrice?.headline, "Best price across 3 venues")
        XCTAssertEqual(bestPrice?.isSell, false)
    }
}

private struct UnusedProposeService: ProposeService {
    struct Unused: Error {}

    func preview(cabalID _: String, draft _: ProposalDraft, network _: Components.Schemas.Network?) async throws
        -> ProposePreview
    { throw Unused() }
    func propose(
        cabalID _: String, draft _: ProposalDraft, network _: Components.Schemas.Network?,
        submission _: IdempotentSubmission
    ) async throws -> String {
        throw Unused()
    }
    func withdraw(proposalID _: String, submission _: IdempotentSubmission) async throws { throw Unused() }
}
