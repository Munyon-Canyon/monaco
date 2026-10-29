import MonacoAnalytics
import MonacoCore
import SwiftUI

nonisolated struct VoteHoldRequest: Identifiable, Hashable, Sendable {
    let id = UUID()
    let proposalID: String
    let pick: ProposalBallot?
}

struct VoteHoldSheet: View {
    static let committedHold: Duration = .milliseconds(600)
    static let settledHold: Duration = .seconds(1)

    let request: VoteHoldRequest
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(\.dismiss) private var dismiss
    @Environment(\.analytics) private var analytics
    @State private var box = VoteHoldBox()
    @State private var holding = false

    var body: some View {
        let model = box.model(request: request, environment: environment)
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            header
            content(model)
            Spacer(minLength: 0)
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.m)
        .presentationDetents([.fraction(MonacoSheetDetents.halfFraction)])
        .presentationDragIndicator(.visible)
        .task {
            await model.load()
            await model.detail.observe()
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("vote-hold-sheet")
    }

    private var header: some View {
        HStack {
            Text("Your vote")
                .font(MonacoTheme.Typo.section)
                .foregroundStyle(MonacoTheme.primaryText)
            Spacer()
            Button {
                dismiss()
            } label: {
                Image(systemName: "xmark.circle.fill")
                    .font(.title2)
                    .foregroundStyle(MonacoTheme.secondaryText)
            }
            .frame(minWidth: CommentRowMetrics.replyTarget, minHeight: CommentRowMetrics.replyTarget)
            .contentShape(Rectangle())
            .accessibilityLabel("Close")
            .accessibilityIdentifier("vote-hold-close")
        }
    }

    @ViewBuilder private func content(_ model: VoteHoldModel) -> some View {
        switch model.phase {
        case .loading:
            ProgressView().frame(maxWidth: .infinity).accessibilityIdentifier("vote-hold-loading")
        case .failed:
            Text(model.detail.errorMessage ?? "Couldn't load this vote.")
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.secondaryText)
                .accessibilityIdentifier("vote-hold-failed")
            Button("Try again") { Task { await model.load() } }
                .buttonStyle(.monacoText)
        default:
            if let summary = model.summary, let tally = model.tally {
                MonacoCard { row(model, summary) }
                card(model, summary, tally)
                Button("Read the thesis") { openProposal() }
                    .buttonStyle(.monacoText)
                    .frame(maxWidth: .infinity)
                    .accessibilityIdentifier("vote-hold-thesis")
            }
        }
    }

    private func row(_ model: VoteHoldModel, _ summary: ProposalSummary) -> some View {
        let proposer = model.detail.members.first { $0.id == summary.proposerID }?.name
        let list = ProposalListRow(
            summary: summary, asset: model.detail.asset, proposer: proposer ?? "Member", now: .now)
        return Button {
            openProposal()
        } label: {
            HStack(spacing: MonacoTheme.Space.sm) {
                StockMark(
                    symbol: summary.symbol, displayName: list.assetName, assetKind: list.kind, size: .row,
                    logoURL: list.logoURL)
                VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                    Text(list.title)
                        .font(MonacoTheme.Typo.headline)
                        .foregroundStyle(MonacoTheme.primaryText)
                    Text([summary.cabalName, proposer].compactMap { $0 }.joined(separator: " · "))
                        .font(MonacoTheme.Typo.subhead)
                        .foregroundStyle(MonacoTheme.secondaryText)
                }
                Spacer(minLength: 0)
            }
        }
        .buttonStyle(.plain)
        .accessibilityIdentifier("vote-hold-row")
    }

    @ViewBuilder private func card(_ model: VoteHoldModel, _ summary: ProposalSummary, _ tally: ProposalTally)
        -> some View
    {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
            VoteTallyBar(yes: tally.yes, no: tally.no, eligible: tally.voters)
            switch model.phase {
            case .ready:
                pair(model, summary, tally)
            case .alreadyVoted(let side):
                let line = ProposalVoteCopy.committedOrdinary(choice: side, tally: tally)
                HStack {
                    Text(line.title)
                        .font(MonacoTheme.Typo.headline)
                        .foregroundStyle(MonacoTheme.primaryText)
                        .accessibilityIdentifier("proposal-voted")
                    Spacer()
                    Button("Change") { model.detail.beginChange() }
                        .buttonStyle(.monacoText)
                        .accessibilityIdentifier("proposal-change-vote")
                }
                VoteStatusLine(result: nil, facts: model.detail.statusFacts, identifierPrefix: "proposal")
            case .closed:
                Text("This vote has closed.")
                    .font(MonacoTheme.Typo.headline)
                    .foregroundStyle(MonacoTheme.primaryText)
                    .accessibilityIdentifier("vote-hold-closed")
                Button("See the result") { openProposal() }
                    .buttonStyle(.monacoText)
                    .accessibilityIdentifier("vote-hold-result")
            case .viewOnly:
                Text(ProposalVoteCopy.readOnlyLabel(cabalName: summary.cabalName))
                    .font(MonacoTheme.Typo.subhead)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .accessibilityIdentifier("vote-hold-view-only")
            case .loading, .failed:
                EmptyView()
            }
        }
    }

    @ViewBuilder private func pair(_ model: VoteHoldModel, _ summary: ProposalSummary, _ tally: ProposalTally)
        -> some View
    {
        let settled = model.detail.lastVote?.settled == true
        VotePair(
            yes: tally.yes, no: tally.no,
            selection: Binding(
                get: { ProposalVoteSection.selection(model.detail.voteSelection) },
                set: { model.detail.voteSelection = ProposalVoteSection.selection($0) }),
            isEnabled: !settled, identifierPrefix: "proposal",
            onCommit: { side in commit(model, summary, side) },
            onPhaseChange: { holding = $0 == .holding })
        if let facts = model.detail.settledFacts {
            VoteStatusLine(result: nil, facts: facts, identifierPrefix: "proposal")
                .accessibilityIdentifier("proposal-vote-settled")
        } else {
            VoteStatusLine(result: nil, facts: model.detail.statusFacts, identifierPrefix: "proposal")
            if case .picked = model.detail.voteSelection {
                Label(
                    ProposalVoteCopy.hint(holding ? .holding : .picked),
                    systemImage: holding ? "hand.tap" : "hand.point.up"
                )
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.secondaryText)
                .accessibilityIdentifier("proposal-vote-hint")
            }
        }
    }

    private func commit(_ model: VoteHoldModel, _ summary: ProposalSummary, _ side: VoteSide) {
        let ballot: ProposalBallot = side == .yes ? .yes : .no
        Task {
            guard await model.commit(ballot), let outcome = model.detail.lastVote else {
                if let message = model.detail.voteRefusal?.message ?? model.detail.errorMessage {
                    toasts.current = MonacoToast(message: message)
                } else {
                    Haptics.warning()
                }
                return
            }
            analytics.step(.vote(.voteCast))
            try? await Task.sleep(for: outcome.settled ? Self.settledHold : Self.committedHold)
            let toast = ProposalVoteCopy.toast(outcome: outcome)
            let action =
                toast.offersUndo
                ? MonacoToastAction(title: "Undo", expiresAt: outcome.undoUntil) { undo(model) }
                : nil
            dismiss()
            toasts.show(success: toast.message, action: action)
            let sample = PushSample.afterVote(
                symbol: summary.symbol, status: outcome.status, tally: outcome.tally, isSell: summary.kind == "sell",
                usdcMicros: summary.usdcMicros, closesAt: summary.expiresAt, swap: summary.swap, pendingUnits: nil)
            await environment.pushPrePrompt.noteFirstVote(sample: sample, after: toasts)
        }
    }

    private func undo(_ model: VoteHoldModel) {
        Task {
            if await model.undo() {
                analytics.step(.vote(.voteUndone))
                toasts.show(success: ProposalVoteCopy.undone)
            } else if let message = model.detail.undoRefusal?.message ?? model.detail.errorMessage {
                toasts.current = MonacoToast(message: message)
            }
        }
    }

    private func openProposal() {
        dismiss()
        environment.navigator.open(ProposalRoute(proposalID: request.proposalID), in: environment.navigator.selectedTab)
    }
}

@MainActor
final class VoteHoldBox {
    private var model: VoteHoldModel?

    func model(request: VoteHoldRequest, environment: AppEnvironment) -> VoteHoldModel {
        if let model { return model }
        let created = VoteHoldModel(
            proposalID: request.proposalID, pick: request.pick, repository: ProposalsRepository(api: environment.api),
            hints: environment.hints)
        model = created
        return created
    }
}
