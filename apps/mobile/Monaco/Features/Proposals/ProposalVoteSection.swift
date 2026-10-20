import MonacoAnalytics
import MonacoCore
import SwiftUI

struct ProposalVoteSection: View {
    let model: ProposalDetailModel
    let summary: ProposalSummary
    @Environment(ToastCenter.self) private var toasts
    @Environment(\.analytics) private var analytics
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @State private var changing = false

    private var state: ProposalState { summary.state(now: .now) }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            MonacoSectionHeader(state.isOpen ? "Your vote" : "Result")
            VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
                content
                caption
            }
            .disabled(model.isVoting)
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("proposal-vote")
    }

    @ViewBuilder private var content: some View {
        switch state {
        case .open:
            let layout =
                dynamicTypeSize.isAccessibilitySize
                ? AnyLayout(VStackLayout(spacing: MonacoTheme.Space.s))
                : AnyLayout(HStackLayout(spacing: MonacoTheme.Space.sm))
            layout {
                Button("Vote Yes") { cast("yes") }
                    .buttonStyle(.monacoPrimary)
                    .accessibilityIdentifier("proposal-vote-yes")
                Button("Vote No") { cast("no") }
                    .buttonStyle(.monacoOutline)
                    .accessibilityIdentifier("proposal-vote-no")
            }
        case .voted(let ballot):
            VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
                Text("You voted \(ballot.label)")
                    .font(MonacoTheme.Typo.headline)
                    .foregroundStyle(MonacoTheme.ink)
                    .accessibilityIdentifier("proposal-voted")
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
            Text("View-only · Voting is restricted to eligible members")
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.muted)
                .fixedSize(horizontal: false, vertical: true)
                .accessibilityIdentifier("proposal-view-only")
        default:
            Text(result)
                .font(MonacoTheme.Typo.body)
                .foregroundStyle(MonacoTheme.bodyText)
                .fixedSize(horizontal: false, vertical: true)
                .accessibilityIdentifier("proposal-vote-result")
        }
    }

    @ViewBuilder private var caption: some View {
        if state.isOpen {
            Text(progress)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
                .fixedSize(horizontal: false, vertical: true)
                .accessibilityIdentifier("proposal-vote-progress")
        }
    }

    private var progress: String {
        let tally = summary.tally
        let votes = tally.needed == 1 ? "vote" : "votes"
        let closes = ProposalCardCopy.closes(at: summary.expiresAt, now: .now)
        return "\(tally.yes) of \(tally.needed) yes \(votes) · \(closes)"
    }

    private var result: String {
        let note = ProposalCardCopy.note(
            state, needed: summary.tally.needed, expiresAt: summary.expiresAt, statusMessage: summary.statusMessage,
            now: .now)
        guard let ballot = summary.myBallot.flatMap(ProposalBallot.init(rawValue:)) else { return note }
        return "You voted \(ballot.label). \(note)"
    }

    private func change(to choice: ProposalBallot, from current: ProposalBallot) {
        guard choice != current else { return }
        cast(choice.rawValue)
    }

    private func cast(_ choice: String) {
        Task {
            if await model.vote(choice) {
                analytics.step(.vote(.voteCast))
                Haptics.success()
                toasts.show(success: "Vote recorded.")
            } else if let message = model.errorMessage {
                toasts.current = MonacoToast(message: message)
            }
        }
    }
}
