import MonacoAPI
import MonacoCore
import SwiftUI

extension View {
    func proposalToasts(_ voting: ProposalVoting) -> some View {
        modifier(ProposalVotingToasts(voting: voting))
    }
}

private struct ProposalVotingToasts: ViewModifier {
    let voting: ProposalVoting

    @Environment(ToastCenter.self) private var toasts

    func body(content: Content) -> some View {
        content.onChange(of: voting.toast) { _, toast in
            guard let toast else { return }
            toasts.current = MonacoToast(message: toast.message, isSuccess: toast.isSuccess)
        }
    }
}

struct ProposalCardStack: View {
    let cards: [ProposalCard]
    let voting: ProposalVoting
    let vote: (BallotChoice, ProposalCard) -> Void

    var body: some View {
        VStack(spacing: MonacoTheme.Space.sm) {
            ForEach(cards) { card in
                ProposalCardView(card: card, isCasting: voting.casting.contains(card.id)) { choice in
                    vote(choice, card)
                }
            }
        }
    }
}
