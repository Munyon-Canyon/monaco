import MonacoAnalytics
import MonacoCore
import SwiftUI

struct HomeVoteCard: View {
    let model: HomeVoteCardModel
    var identifierPrefix = "home"
    var tab: MainTab = .home
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @State private var holding = false

    private var row: HomePendingVote { model.row }
    private var tally: ProposalTally { model.tally }

    var body: some View {
        MonacoCard {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
                header
                VoteTallyBar(yes: tally.yes, no: tally.no, eligible: tally.voters)
                if let outcome = model.outcome, outcome.settled {
                    settledLine(outcome)
                } else {
                    buttons
                    hint
                }
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("\(identifierPrefix)-vote-card-\(model.id)")
        .task { await model.observe() }
        .onChange(of: model.phase) { _, new in phaseChanged(new) }
    }

    private var header: some View {
        Button {
            environment.navigator.open(ProposalRoute(proposalID: model.id), in: tab)
        } label: {
            HStack(spacing: MonacoTheme.Space.sm) {
                if !row.isRule {
                    StockMark(symbol: row.symbol ?? "", displayName: row.assetName, logoURL: row.logoURL)
                }
                VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                    Text(model.headline)
                        .font(MonacoTheme.Typo.headline)
                        .foregroundStyle(MonacoTheme.primaryText)
                        .accessibilityIdentifier("\(identifierPrefix)-vote-headline-\(model.id)")
                    Text(model.subhead(now: .now))
                        .font(MonacoTheme.Typo.subhead)
                        .foregroundStyle(MonacoTheme.secondaryText)
                        .accessibilityIdentifier("\(identifierPrefix)-vote-subhead-\(model.id)")
                }
                .frame(maxWidth: .infinity, alignment: .leading)
                Image(monaco: .chevron)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .accessibilityHidden(true)
            }
            .frame(minHeight: MonacoButtonMetrics.hitTarget)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityHint("Opens the proposal")
        .accessibilityIdentifier("\(identifierPrefix)-vote-open-\(model.id)")
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

    private static func accepted(
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
        toasts.show(success: toast.message, action: action)
        let row = card.row
        guard !row.isRule, let symbol = row.symbol else { return }
        let sample = PushSample.afterVote(
            symbol: symbol, status: outcome.status, tally: outcome.tally, isSell: row.isSell,
            usdcMicros: row.usdcMicros, closesAt: row.expiresAt, swap: nil, pendingUnits: nil)
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
