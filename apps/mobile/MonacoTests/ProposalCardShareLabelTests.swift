import MonacoAPI
import Testing

@testable import Monaco
@testable import MonacoCore

struct ProposalCardShareLabelTests {
    private static func sell(atomics: Int64) -> ProposalSummary {
        var proposal = Components.Schemas.Proposal.sample(kind: .sell)
        proposal.tokenAmount = atomics
        return ProposalSummary(proposal, canVote: true)
    }

    private static let stock = ProposalAsset(.googl)

    @Test func aFractionalSellReadsAsShares() {
        let label = ProposalCard.shareLabel(summary: Self.sell(atomics: 21_610_000), asset: Self.stock)
        #expect(label == "0.2161 shares")
        #expect(!label.contains("ReversedCollection"))
    }

    @Test func oneWholeShareReadsAsOneShare() {
        #expect(ProposalCard.shareLabel(summary: Self.sell(atomics: 100_000_000), asset: Self.stock) == "1 share")
    }
}
