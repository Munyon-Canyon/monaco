import MonacoCore
import SwiftUI

struct ProposalVoteCard: View {
    let proposal: ProposalSummary
    let voting: ProposalVoteModel
    var asset: ProposalAsset?
    var members: [ProposalMember] = []
    var paused = false
    var onVoted: () async -> Void = {}
    @Environment(ToastCenter.self) private var toasts

    var body: some View {
        ProposalCard(
            proposal: voting.applying(proposal), asset: asset, members: members, paused: paused,
            openRoute: AnyAppRoute(ProposalRoute(proposalID: proposal.id))
        ) { choice in
            Task {
                if await voting.vote(choice, on: proposal) {
                    toasts.show(success: "Vote in")
                    await onVoted()
                } else if let message = voting.errorMessage {
                    toasts.current = MonacoToast(message: message)
                }
            }
        }
    }
}
