import Foundation
import MonacoAPI
import Testing

@testable import Monaco
@testable import MonacoCore

struct ProposalListRowTests {
    private static func summary(
        status: Components.Schemas.ProposalStatus = .open, kind: Components.Schemas.ProposalKind = .buy
    ) -> ProposalSummary {
        var proposal = Components.Schemas.Proposal.sample(status: status, kind: kind)
        proposal.tokenAmount = kind == .sell ? 150_000_000 : nil
        proposal.usdcMicros = kind == .sell ? nil : 250_000_000
        return ProposalSummary(proposal, canVote: true)
    }

    @Test func anOpenBuyReadsAsAnOpenRowWithItsAmountAndProposer() {
        let row = ProposalListRow(summary: Self.summary(), asset: nil, proposer: "Mia", now: .now)
        #expect(row.title == "AAPL · $250.00")
        #expect(row.subtitle == "Mia · Voting open")
        #expect(row.badge == .listRow(.open))
        #expect(row.id == "proposal-1")
    }

    @Test func aPassedProposalReadsAsPendingNotOpen() {
        let row = ProposalListRow(summary: Self.summary(status: .passed), asset: nil, proposer: "Mia", now: .now)
        #expect(row.subtitle == "Approved · execution pending")
        #expect(row.badge.label == "Pending")
        #expect(row.badge.tone == .warn)
    }

    @Test func aSellLeadsWithItsShares() {
        let row = ProposalListRow(
            summary: Self.summary(status: .expired, kind: .sell), asset: nil, proposer: "Mia", now: .now)
        #expect(row.title == "AAPL · 1.5 shares")
        #expect(row.badge.label == "Expired")
    }
}
