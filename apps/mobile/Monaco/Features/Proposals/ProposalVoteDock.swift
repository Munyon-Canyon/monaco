import MonacoAnalytics
import MonacoCore
import SwiftUI

struct ProposalVoteDock: View {
    let model: ProposalDetailModel?
    let comments: CommentsModel?
    @Environment(ToastCenter.self) private var toasts
    @Environment(\.analytics) private var analytics
    @State private var changing = false

    private var state: ProposalState? { model?.summary?.state(now: .now) }

    var body: some View {
        VStack(spacing: 0) {
            actions
                .disabled(model?.isVoting == true)
            if let comments { CommentComposerBar(model: comments) }
        }
    }

    @ViewBuilder private var actions: some View {
        switch state {
        case .open:
            BottomCTA {
                Button("Vote No") { cast("no") }
                    .buttonStyle(.monacoOutline)
                    .accessibilityIdentifier("proposal-vote-no")
                Button("Vote Yes") { cast("yes") }
                    .buttonStyle(.monacoPrimary)
                    .accessibilityIdentifier("proposal-vote-yes")
            }
        case .voted(let ballot):
            BottomCTA {
                Button("Change vote") { changing = true }
                    .buttonStyle(.monacoOutline)
                    .accessibilityIdentifier("proposal-change-vote")
            }
            .confirmationDialog("Change vote", isPresented: $changing, titleVisibility: .visible) {
                Button("Vote Yes") { change(to: .yes, from: ballot) }
                Button("Vote No") { change(to: .no, from: ballot) }
                Button("Cancel", role: .cancel) {}
            } message: {
                Text("Your current vote: \(ballot.label)")
            }
        case .viewOnly:
            BottomCTA {
                Text("View-only · Voting is restricted to eligible members")
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
                    .multilineTextAlignment(.center)
                    .frame(maxWidth: .infinity)
                    .accessibilityIdentifier("proposal-view-only")
            }
        default:
            EmptyView()
        }
    }

    private func change(to choice: ProposalBallot, from current: ProposalBallot) {
        guard choice != current else { return }
        cast(choice.rawValue)
    }

    private func cast(_ choice: String) {
        guard let model else { return }
        Task {
            if await model.vote(choice) {
                analytics.step(.vote(.voteCast))
                toasts.show(success: "Vote recorded.")
            } else if let message = model.errorMessage {
                toasts.current = MonacoToast(message: message)
            }
        }
    }
}
