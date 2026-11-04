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
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    private var detents: Set<PresentationDetent> {
        let half = PresentationDetent.fraction(MonacoSheetDetents.halfFraction)
        return dynamicTypeSize >= .xxxLarge ? [half, .large] : [half]
    }

    var body: some View {
        let model = box.model(request: request, environment: environment)
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                header
                content(model)
                Spacer(minLength: 0)
            }
        }
        .scrollBounceBehavior(.basedOnSize)
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.m)
        .presentationDetents(detents)
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
                .fixedSize(horizontal: false, vertical: true)
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
                voteCard(model, summary, tally)
                Button("Read the thesis") { openProposal() }
                    .buttonStyle(.monacoText)
                    .frame(maxWidth: .infinity)
                    .accessibilityIdentifier("vote-hold-thesis")
            }
        }
    }

    static func title(_ summary: ProposalSummary) -> String {
        if summary.isRule { return ProposalRuleCopy.voteTitle }
        return title(kind: summary.kind, symbol: summary.symbol, usdcMicros: summary.usdcMicros)
    }

    static func title(kind: String, symbol: String, usdcMicros: Int64?) -> String {
        let side = kind == "sell" ? "Sell" : "Buy"
        let head = "\(side) \(AssetSymbolFormatter.display(symbol))"
        guard let micros = usdcMicros else { return head }
        let amount = UsdAmountFormatter.format(micros: micros)
        return "\(head) \u{00B7} \(amount.hasSuffix(".00") ? String(amount.dropLast(3)) : amount)"
    }

    static func line(_ summary: ProposalSummary, tally: ProposalTally, replies: Int?, now: Date) -> VoteCardLine {
        ProposalVoteCopy.cardLine(
            cabalName: summary.cabalName, tally: tally, replies: replies, expiresAt: summary.expiresAt, now: now)
    }

    private func voteCard(_ model: VoteHoldModel, _ summary: ProposalSummary, _ tally: ProposalTally) -> some View {
        let list = ProposalListRow(summary: summary, asset: model.detail.asset, proposer: "", now: .now)
        return VoteCard(
            title: Self.title(summary),
            mark: summary.isRule ? .rule : .stock(symbol: summary.symbol, name: list.assetName, logoURL: list.logoURL),
            line: Self.line(summary, tally: tally, replies: model.detail.value?.replyCount, now: .now),
            yes: tally.yes, no: tally.no, eligible: tally.voters,
            identifierPrefix: "vote-hold", identifierSuffix: nil, onOpen: openProposal
        ) {
            if let change = summary.ruleChange, summary.isRule {
                Text(change.detailLine)
                    .font(MonacoTheme.Typo.subhead)
                    .foregroundStyle(MonacoTheme.primaryText)
                    .fixedSize(horizontal: false, vertical: true)
                    .accessibilityIdentifier("vote-hold-rule-change")
            }
            actions(model, summary, tally)
        }
        .accessibilityIdentifier("vote-hold-row")
    }

    @ViewBuilder private func actions(_ model: VoteHoldModel, _ summary: ProposalSummary, _ tally: ProposalTally)
        -> some View
    {
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
            if case .picked = model.detail.voteSelection {
                Label(
                    ProposalVoteCopy.hint(holding ? .holding : .picked),
                    systemImage: holding ? "hand.tap" : "hand.point.up"
                )
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.secondaryText)
                .fixedSize(horizontal: false, vertical: true)
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
            toasts.show(vote: toast, action: action)
            let sample = PushSample.afterVote(
                symbol: summary.symbol, status: outcome.status, tally: outcome.tally, isSell: summary.kind == "sell",
                usdcMicros: summary.usdcMicros, closesAt: summary.expiresAt, swap: summary.swap, pendingUnits: nil,
                isRule: summary.isRule,
                callerName: HomePendingVote.callerName(
                    proposerID: summary.proposerID,
                    name: model.detail.members.first { $0.id == summary.proposerID }?.name,
                    viewerID: environment.viewer?.userID))
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
