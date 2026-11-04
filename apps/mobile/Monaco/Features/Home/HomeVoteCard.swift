import MonacoAnalytics
import MonacoCore
import SwiftUI

struct HomeVoteCard: View {
    let model: HomeVoteCardModel
    var identifierPrefix = "home"
    var tab: MainTab = .home
    var compact = false
    var inCabal = false
    var canVote = true
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @State private var holding = false
    @State private var collapsed: Bool

    static let collapseDelay: TimeInterval = 1

    init(
        model: HomeVoteCardModel, identifierPrefix: String = "home", tab: MainTab = .home, compact: Bool = false,
        inCabal: Bool = false, canVote: Bool = true
    ) {
        self.model = model
        self.identifierPrefix = identifierPrefix
        self.tab = tab
        self.compact = compact
        self.inCabal = inCabal
        self.canVote = canVote
        _collapsed = State(initialValue: Self.hasCollapsed(model, now: .now))
    }

    static func hasCollapsed(_ model: HomeVoteCardModel, now: Date) -> Bool {
        guard model.isSettled, let committedAt = model.committedAt else { return false }
        return now.timeIntervalSince(committedAt) >= collapseDelay
    }

    static func collapseDelayAfterAppear(isSettled: Bool, collapsed: Bool) -> TimeInterval? {
        isSettled && !collapsed ? collapseDelay : nil
    }

    private var line: VoteCardLine {
        let line = model.subheadLine(now: .now, showsCabal: !inCabal)
        return compact ? VoteCardLine(full: line.short, short: line.short) : line
    }

    private var row: HomePendingVote { model.row }
    private var tally: ProposalTally { model.tally }

    var body: some View {
        VoteCard(
            title: inCabal && row.isRule ? "Rule change" : model.headline,
            mark: row.isRule ? .rule : .stock(symbol: row.symbol ?? "", name: row.assetName, logoURL: row.logoURL),
            line: line,
            yes: tally.yes, no: tally.no, eligible: tally.voters,
            identifierPrefix: identifierPrefix, identifierSuffix: model.id, showsChevron: !compact,
            ruleIcon: inCabal ? .eye : .thesis,
            onOpen: { environment.navigator.open(ProposalRoute(proposalID: model.id), in: tab) }
        ) {
            if let outcome = model.outcome, outcome.settled, collapsed {
                settledLine(outcome)
            } else if canVote || model.isCommitted {
                buttons
                hint
            }
        }
        .task { await model.observe() }
        .task(id: model.isSettled) { await collapseWhenDue() }
        .onChange(of: model.isSettled) { _, settled in if !settled { collapsed = false } }
        .onChange(of: model.phase) { _, new in phaseChanged(new) }
    }

    private func collapseWhenDue() async {
        guard let delay = Self.collapseDelayAfterAppear(isSettled: model.isSettled, collapsed: collapsed) else {
            return
        }
        try? await Task.sleep(for: .seconds(delay))
        guard !Task.isCancelled else { return }
        collapsed = true
    }

    private func settledLine(_ outcome: CastOutcome) -> some View {
        let line = ProposalVoteCopy.committedOrdinary(choice: outcome.choice, tally: outcome.tally)
        return HStack(spacing: MonacoTheme.Space.sm) {
            Image(systemName: outcome.status == .passed ? "checkmark.seal.fill" : "xmark.circle")
                .foregroundStyle(outcome.status == .passed ? MonacoTheme.profit : MonacoTheme.loss)
                .accessibilityHidden(true)
            Text(line.title)
                .font(MonacoTheme.Typo.headline)
                .foregroundStyle(MonacoTheme.primaryText)
            Label("\(line.yes)", systemImage: "checkmark")
                .foregroundStyle(MonacoTheme.profit)
            Label("\(line.no)", systemImage: "xmark")
                .foregroundStyle(MonacoTheme.loss)
            Spacer(minLength: 0)
        }
        .font(MonacoTheme.Typo.subhead)
        .frame(minHeight: MonacoButtonMetrics.hitTarget)
        .contentShape(.accessibility, Rectangle())
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(line.accessibilityLabel)
        .accessibilityIdentifier("\(identifierPrefix)-vote-settled-\(model.id)")
    }

    @ViewBuilder private var buttons: some View {
        if case .holding(let side, let progress) = model.phase, progress > 0 {
            VoteHoldRow(
                yes: frozen(.yes, held: side.voteSide, progress: progress),
                no: frozen(.no, held: side.voteSide, progress: progress))
        } else {
            VotePair(
                yes: tally.yes, no: tally.no, selection: selection, isEnabled: true, identifierPrefix: identifierPrefix,
                identifierSuffix: model.id, onCommit: commit, onPhaseChange: { holding = $0 == .holding })
        }
    }

    private func frozen(_ side: VoteSide, held: VoteSide, progress: Double) -> some View {
        VoteHoldButton(
            side: side, count: side == .yes ? tally.yes : tally.no,
            phase: VotePair.buttonPhase(
                side, selection: .picked(held), holding: held, progress: progress, isEnabled: true)
        )
        .accessibilityIdentifier(VoteIdentifier.make(identifierPrefix, "vote-\(side.title.lowercased())", model.id))
        .accessibilityActions { Button("Vote \(side.title)") { commit(side) } }
    }

    @ViewBuilder private var hint: some View {
        if let text = model.hint(holding: holding) {
            if model.isCommitted {
                Text(text)
                    .font(MonacoTheme.Typo.subhead)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .fixedSize(horizontal: false, vertical: true)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .accessibilityIdentifier("\(identifierPrefix)-vote-deciding-\(model.id)")
            } else {
                Label(text, systemImage: holding ? "hand.tap" : "hand.point.up")
                    .font(MonacoTheme.Typo.subhead)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .accessibilityIdentifier("\(identifierPrefix)-vote-hint-\(model.id)")
            }
        }
    }

    private var selection: Binding<VoteSelection> {
        Binding(
            get: { Self.selection(model.phase) },
            set: { apply($0) })
    }

    static func selection(_ phase: HomeVoteCardModel.Phase) -> VoteSelection {
        switch phase {
        case .idle, .failed: .none
        case .picked(let side), .holding(let side, _): .picked(side.voteSide)
        case .committing(let side): .pending(side.voteSide)
        case .committed(let outcome): .committed(outcome.choice.voteSide)
        }
    }

    private func apply(_ value: VoteSelection) {
        switch value {
        case .none: model.select(nil)
        case .picked(let side): model.select(side.ballot)
        case .pending(let side): model.beginCommit(side.ballot)
        case .committed: break
        }
    }

    private func commit(_ side: VoteSide) {
        Task { await model.vote(side.ballot) }
    }

    private func phaseChanged(_ phase: HomeVoteCardModel.Phase) {
        switch phase {
        case .failed(let message): refused(message)
        case .idle, .picked, .holding, .committing, .committed: break
        }
    }

    private func refused(_ message: String) {
        toasts.current = MonacoToast(message: message)
        model.dismissFailure()
    }
}

@MainActor
enum HomeVoteCommit {
    static func install(
        on votes: HomePendingVotesModel, environment: AppEnvironment, toasts: ToastCenter, analytics: Analytics
    ) {
        votes.onVoteCommitted = { card, outcome in
            accepted(card, outcome, environment: environment, toasts: toasts, analytics: analytics)
        }
    }

    static func accepted(
        _ card: HomeVoteCardModel, _ outcome: CastOutcome, environment: AppEnvironment, toasts: ToastCenter,
        analytics: Analytics
    ) {
        analytics.step(.vote(.voteCast))
        let toast = ProposalVoteCopy.toast(outcome: outcome)
        let action =
            toast.offersUndo
            ? MonacoToastAction(title: "Undo", expiresAt: outcome.undoUntil) {
                undo(card, toasts: toasts, analytics: analytics)
            }
            : nil
        toasts.show(vote: toast, action: action)
        let row = card.row
        guard !row.isRule, let symbol = row.symbol else { return }
        let sample = PushSample.afterVote(
            symbol: symbol, status: outcome.status, tally: outcome.tally, isSell: row.isSell,
            usdcMicros: row.usdcMicros, closesAt: row.expiresAt, swap: nil, pendingUnits: nil,
            callerName: HomePendingVote.callerName(
                proposerID: row.proposerID, name: row.proposerName, viewerID: environment.viewer?.userID))
        Task { await environment.pushPrePrompt.noteFirstVote(sample: sample, after: toasts) }
    }

    private static func undo(_ card: HomeVoteCardModel, toasts: ToastCenter, analytics: Analytics) {
        Task {
            if await card.undo() {
                analytics.step(.vote(.voteUndone))
                toasts.show(success: ProposalVoteCopy.undone)
            } else if let message = card.errorMessage {
                toasts.current = MonacoToast(message: message)
            }
        }
    }
}

extension ProposalBallot {
    fileprivate var voteSide: VoteSide { self == .yes ? .yes : .no }
}

extension VoteSide {
    fileprivate var ballot: ProposalBallot { self == .yes ? .yes : .no }
}
