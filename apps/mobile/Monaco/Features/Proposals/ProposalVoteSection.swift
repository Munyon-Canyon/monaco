import MonacoAnalytics
import MonacoCore
import SwiftUI

struct ProposalVoteSection: View {
    enum Card: Equatable {
        case hidden
        case readOnly
        case pair
        case voted(ProposalBallot)
    }

    static let committedHold: Duration = .milliseconds(1_200)

    let model: ProposalDetailModel
    let summary: ProposalSummary
    var haptics: VoteHaptics = .live
    var warning: @MainActor () -> Void = Haptics.warning
    var viewerID: String?
    @Environment(ToastCenter.self) private var toasts
    @Environment(AppEnvironment.self) private var environment: AppEnvironment?
    @Environment(\.analytics) private var analytics
    @State private var holding = false
    @State private var showsCommitted = false

    private var state: ProposalState { summary.state(now: .now) }
    private var viewer: String? { viewerID ?? environment?.viewer?.userID }
    private var role: ProposalViewerRole { model.viewerRole(viewerID: viewer) }
    private var isSettled: Bool { model.lastVote?.settled == true }

    static func card(
        isOpen: Bool, isSettled: Bool, role: ProposalViewerRole, selection: ProposalVoteSelection, showsCommitted: Bool
    ) -> Card {
        if isSettled { return .pair }
        guard isOpen else { return .hidden }
        guard role == .voter else { return .readOnly }
        if case .committed(let side) = selection, !showsCommitted { return .voted(side) }
        return .pair
    }

    private var card: Card {
        Self.card(
            isOpen: state.isOpen, isSettled: isSettled, role: role, selection: model.voteSelection,
            showsCommitted: showsCommitted)
    }

    var body: some View {
        content
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .onChange(of: model.voteSelection) { old, new in
                if case .pending = old, case .committed = new { showsCommitted = true }
            }
            .task(id: showsCommitted) {
                guard showsCommitted else { return }
                try? await Task.sleep(for: Self.committedHold)
                showsCommitted = false
            }
    }

    @ViewBuilder private var content: some View {
        switch card {
        case .hidden:
            EmptyView()
        case .readOnly:
            readOnly
        case .pair:
            pair
        case .voted(let side):
            voted(side)
        }
    }

    private var tally: ProposalTally { summary.tally }
    private var voters: Int { tally.voters }

    private var selection: Binding<VoteSelection> {
        Binding(
            get: { Self.selection(model.voteSelection) },
            set: { model.voteSelection = Self.selection($0) })
    }

    static func selection(_ value: ProposalVoteSelection) -> VoteSelection {
        switch value {
        case .none: .none
        case .picked(let side): .picked(side.voteSide)
        case .pending(let side): .pending(side.voteSide)
        case .committed(let side): .committed(side.voteSide)
        }
    }

    static func selection(_ value: VoteSelection) -> ProposalVoteSelection {
        switch value {
        case .none: .none
        case .picked(let side): .picked(side.ballot)
        case .pending(let side): .pending(side.ballot)
        case .committed(let side): .committed(side.ballot)
        }
    }

    private var readOnly: some View {
        let middle =
            role == .memberNotVoter
            ? ProposalVoteCopy.readOnlyVoterLabel(cabalName: summary.cabalName)
            : ProposalVoteCopy.readOnlyLabel(cabalName: summary.cabalName)
        return MonacoCard {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
                VoteTallyBar(yes: tally.yes, no: tally.no, eligible: voters)
                VoteTallyEnds(yes: tally.yes, no: tally.no, middle: middle, identifierPrefix: "proposal")
                    .padding(.vertical, MonacoTheme.Space.s)
                VoteStatusLine(result: nil, facts: model.statusFacts, identifierPrefix: "proposal")
            }
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(ProposalVoteCopy.readOnlyAccessibilityLabel(tally: tally, middle: middle))
        .accessibilityIdentifier("proposal-vote-readonly")
    }

    private var pair: some View {
        MonacoCard {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
                VoteTallyBar(yes: tally.yes, no: tally.no, eligible: voters)
                VotePair(
                    yes: tally.yes, no: tally.no, selection: selection, isEnabled: !isSettled,
                    identifierPrefix: "proposal", onCommit: commit,
                    onPhaseChange: { holding = $0 == .holding }, haptics: haptics)
                if let facts = model.settledFacts {
                    settled(facts)
                } else {
                    VoteStatusLine(result: nil, facts: model.statusFacts, identifierPrefix: "proposal")
                    hint
                }
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("proposal-vote")
    }

    @ViewBuilder private func settled(_ facts: [String]) -> some View {
        VoteStatusLine(result: settledMark, facts: facts, identifierPrefix: "proposal")
            .accessibilityIdentifier("proposal-vote-settled")
        if let deciding = model.decidingLine {
            Text(deciding)
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.secondaryText)
                .fixedSize(horizontal: false, vertical: true)
                .frame(maxWidth: .infinity, alignment: .leading)
                .accessibilityIdentifier("proposal-vote-deciding")
        }
    }

    private var settledMark: VoteResultMark {
        guard model.lastVote?.status == .passed else { return .failed }
        return summary.status == .executed ? .passed : .executing
    }

    @ViewBuilder private var hint: some View {
        if case .picked = model.voteSelection {
            Label(
                ProposalVoteCopy.hint(holding ? .holding : .picked),
                systemImage: holding ? "hand.tap" : "hand.point.up"
            )
            .font(MonacoTheme.Typo.subhead)
            .foregroundStyle(MonacoTheme.secondaryText)
            .frame(maxWidth: .infinity, alignment: .leading)
            .accessibilityIdentifier("proposal-vote-hint")
        }
    }

    private func voted(_ side: ProposalBallot) -> some View {
        let line = ProposalVoteCopy.committedOrdinary(choice: side, tally: tally)
        return MonacoCard {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
                VoteTallyBar(yes: tally.yes, no: tally.no, eligible: voters)
                HStack(spacing: MonacoTheme.Space.sm) {
                    Image(systemName: "checkmark.seal.fill")
                        .foregroundStyle(MonacoTheme.profit)
                        .accessibilityHidden(true)
                    Text(line.title)
                        .font(MonacoTheme.Typo.headline)
                        .foregroundStyle(MonacoTheme.primaryText)
                        .accessibilityIdentifier("proposal-voted")
                    Label("\(line.yes)", systemImage: "checkmark")
                        .foregroundStyle(MonacoTheme.profit)
                    Label("\(line.no)", systemImage: "xmark")
                        .foregroundStyle(MonacoTheme.loss)
                    Spacer(minLength: MonacoTheme.Space.sm)
                    Button("Change") { model.beginChange() }
                        .buttonStyle(.monacoText)
                        .accessibilityIdentifier("proposal-change-vote")
                }
                .font(MonacoTheme.Typo.subhead)
                .accessibilityElement(children: .contain)
                .accessibilityLabel(line.accessibilityLabel)
                VoteStatusLine(result: nil, facts: model.statusFacts, identifierPrefix: "proposal")
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("proposal-vote")
    }

    private func commit(_ side: VoteSide) {
        Task {
            if await model.vote(side.ballot.rawValue) {
                accepted()
            } else {
                refused()
            }
        }
    }

    private func accepted() {
        analytics.step(.vote(.voteCast))
        guard let outcome = model.lastVote else { return }
        let toast = ProposalVoteCopy.toast(outcome: outcome)
        let action =
            toast.offersUndo
            ? MonacoToastAction(title: "Undo", expiresAt: outcome.undoUntil) { undo() }
            : nil
        toasts.show(success: toast.message, action: action)
    }

    private func refused() {
        warning()
        if let message = model.voteRefusal?.message ?? model.errorMessage {
            toasts.current = MonacoToast(message: message)
        }
    }

    private func undo() {
        Task {
            if await model.undo() {
                analytics.step(.vote(.voteUndone))
                toasts.show(success: ProposalVoteCopy.undone)
            } else if let message = model.undoRefusal?.message ?? model.errorMessage {
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
