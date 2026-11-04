import MonacoAPI
import MonacoCore
import SwiftUI
import Testing

@testable import Monaco

@MainActor
struct ChatProposalCardTests {
    private func summary(status: Components.Schemas.ProposalStatus) -> ProposalSummary {
        ProposalSummary(Components.Schemas.Proposal.sample(status: status))
    }

    @Test func aClosedProposalReadsItsResultNotACloseTime() {
        let expired = ChatProposalCard<EmptyView, EmptyView, EmptyView, EmptyView>.resultLine(summary(status: .expired))

        #expect(expired.contains("no trade"))
    }

    @Test func aWithdrawnProposalFallsBackToItsStatusLabel() {
        let line = ChatProposalCard<EmptyView, EmptyView, EmptyView, EmptyView>.resultLine(
            summary(status: .withdrawn))

        #expect(line == "Withdrawn")
    }

    @Test func theVoteButtonsCarryTheChatIdentifiers() {
        #expect(VoteIdentifier.make("chat-proposal", "vote-yes", "p1") == "chat-proposal-yes-p1")
        #expect(VoteIdentifier.make("chat-proposal", "vote-no", "p1") == "chat-proposal-no-p1")
    }
}
