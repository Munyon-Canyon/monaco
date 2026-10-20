import MonacoCore
import SwiftUI

struct ProposalVoteCard: View {
    let proposal: ProposalSummary
    let voting: ProposalVoteModel
    var asset: ProposalAsset?
    var members: [ProposalMember] = []
    var paused = false
    var onVoted: () async -> Void = {}

    var body: some View {
        ProposalCard(
            proposal: voting.applying(proposal), asset: asset, members: members, paused: paused,
            openRoute: AnyAppRoute(ProposalRoute(proposalID: proposal.id)))
    }
}
