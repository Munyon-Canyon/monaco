import MonacoAPI
import MonacoCore
import SwiftUI

struct ChatProposalCard<Name: View, Thesis: View, Face: View, Replies: View>: View {
    static var identifierPrefix: String { "chat-proposal" }

    let message: ChatMessage
    let proposalID: String
    let store: ChatProposalStore
    let columnInset: CGFloat
    let isMine: Bool
    let openProposal: () -> Void
    @ViewBuilder let name: () -> Name
    @ViewBuilder let thesis: () -> Thesis
    @ViewBuilder let face: () -> Face
    @ViewBuilder let replies: () -> Replies

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(\.analytics) private var analytics
    @Environment(\.hostMainTab) private var hostMainTab
    @State private var cards = CabalVoteCards()

    private var state: ChatProposalState { store.state(for: proposalID) }

    var body: some View {
        VStack(alignment: isMine ? .trailing : .leading, spacing: MonacoTheme.Space.xs) {
            name().padding(.leading, columnInset + ChatBubbleMetrics.nameInset)
            if !(message.body ?? "").isEmpty { thesis().padding(.leading, columnInset) }
            HStack(alignment: .bottom, spacing: MonacoTheme.Space.s) {
                face()
                content.frame(maxWidth: .infinity)
            }
            replies().padding(.leading, columnInset)
        }
        .padding(.vertical, MonacoTheme.Space.sm)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("chat-proposal-\(message.id)")
        .task(id: proposalID) { await store.load(proposalID) }
    }

    @ViewBuilder private var content: some View {
        switch state {
        case .loading:
            SkeletonBlock(height: 120, radius: MonacoTheme.Radius.card)
                .accessibilityElement(children: .ignore)
                .accessibilityLabel("Loading proposal")
                .accessibilityIdentifier("chat-proposal-loading-\(message.id)")
        case .failed:
            MonacoErrorRow(thing: "proposal", identifier: "chat-proposal-error-\(message.id)", inset: false) {
                Task { await store.retry(proposalID) }
            }
        case .loaded(let facts):
            loaded(facts)
        }
    }

    @ViewBuilder private func loaded(_ facts: ChatProposalFacts) -> some View {
        let summary = facts.summary
        let model = card(for: summary, asset: facts.asset)
        if summary.status == .open {
            HomeVoteCard(
                model: model, identifierPrefix: Self.identifierPrefix,
                tab: hostMainTab ?? environment.navigator.selectedTab, inCabal: true, canVote: summary.canVote)
        } else {
            let result = Self.resultLine(summary)
            VoteCard(
                title: model.headline,
                mark: summary.isRule
                    ? .rule
                    : .stock(symbol: summary.symbol, name: facts.asset?.displayName, logoURL: facts.asset?.logoURL),
                line: VoteCardLine(full: result, short: result),
                yes: summary.tally.yes, no: summary.tally.no, eligible: summary.tally.voters,
                identifierPrefix: Self.identifierPrefix, identifierSuffix: summary.id, onOpen: openProposal
            ) {
                EmptyView()
            }
        }
    }

    static func resultLine(_ summary: ProposalSummary) -> String {
        let caption = ProposalResultCopy.caption(
            status: summary.status, tally: summary.tally, swap: summary.swap, isSell: summary.kind == "sell",
            pendingUnits: nil, statusMessage: summary.statusMessage, isRule: summary.isRule)
        return caption
            ?? ProposalChip.label(status: summary.status, isSell: summary.kind == "sell", isRule: summary.isRule)
            ?? "Closed"
    }

    private func card(for summary: ProposalSummary, asset: ProposalAsset?) -> HomeVoteCardModel {
        let row = CabalProposals.pendingVote(summary, asset: asset)
        let id = summary.id
        return cards.card(
            for: row, ballot: summary.myBallot.flatMap(ProposalBallot.init(rawValue:)), environment: environment,
            showsReplies: false
        ) { [environment, toasts, analytics, store] card, outcome in
            HomeVoteCommit.accepted(card, outcome, environment: environment, toasts: toasts, analytics: analytics)
            Task { await store.reload([id]) }
        }
    }
}
