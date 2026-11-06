import MonacoAPI
import MonacoCore
import SwiftUI

struct ChatPinListLayout: Equatable {
    let voteCount: Int
    let pinCount: Int

    var showsVotes: Bool { voteCount > 0 }
    var showsMessages: Bool { pinCount > 0 }
    var showsMessagesHeader: Bool { showsVotes && showsMessages }
    var isEmpty: Bool { voteCount == 0 && pinCount == 0 }

    static func countLabel(_ count: Int) -> String { "\(count)" }
}

struct ChatPinListSheet: View {
    let pins: ChatPinsModel
    let cabalName: String
    let viewerID: String
    let controls: ChatPinControls
    var openVotes: ChatOpenVotesModel?
    var proposals: ChatProposalStore?
    var goToVote: (String) -> Void = { _ in }
    let open: (String) -> Void

    @Environment(ToastCenter.self) private var toasts
    @Environment(AppEnvironment.self) private var environment
    @Environment(\.analytics) private var analytics
    @Environment(\.hostMainTab) private var hostMainTab
    @State private var cards = CabalVoteCards()

    private var votes: [ProposalSummary] { openVotes?.votes ?? [] }
    private var layout: ChatPinListLayout { ChatPinListLayout(voteCount: votes.count, pinCount: pins.pins.count) }

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(spacing: 0) {
                    if layout.showsVotes { votesSection }
                    if layout.showsMessages { messagesSection }
                }
            }
            .monacoSheet(title: ChatPinCopy.sheetTitle, titleIdentifier: "chat-pin-list-title", detents: .votes)
            .toolbar(.hidden, for: .navigationBar)
        }
        .accessibilityIdentifier("chat-pin-list")
    }

    private var votesSection: some View {
        VStack(spacing: 0) {
            header(ChatOpenVotesModel.sectionTitle, count: votes.count)
                .accessibilityIdentifier("chat-pin-list-votes")
            ForEach(votes, id: \.id) { summary in
                voteCard(summary)
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                    .padding(.bottom, MonacoTheme.Space.sm)
                    .accessibilityIdentifier("chat-pin-list-vote-\(summary.id)")
            }
        }
    }

    private var messagesSection: some View {
        VStack(spacing: 0) {
            if layout.showsMessagesHeader {
                header(ChatPinCopy.messagesTitle, count: nil)
                    .accessibilityIdentifier("chat-pin-list-messages")
            }
            ForEach(pins.pins, id: \.id) { message in
                row(message)
            }
        }
    }

    private func header(_ title: String, count: Int?) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: MonacoTheme.Space.s) {
            Text(title)
                .font(.title3.weight(.semibold))
                .foregroundStyle(MonacoTheme.primaryText)
            if let count {
                Text(ChatPinListLayout.countLabel(count))
                    .font(MonacoTheme.Typo.body)
                    .foregroundStyle(MonacoTheme.secondaryText)
            }
            Spacer(minLength: 0)
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.sm)
        .padding(.bottom, MonacoTheme.Space.s)
        .accessibilityElement(children: .combine)
        .accessibilityAddTraits(.isHeader)
    }

    private func voteCard(_ summary: ProposalSummary) -> some View {
        var asset: ProposalAsset?
        if case .loaded(let facts) = proposals?.state(for: summary.id) { asset = facts.asset }
        let row = CabalProposals.pendingVote(summary, asset: asset)
        let id = summary.id
        let model = cards.card(
            for: row, ballot: summary.myBallot.flatMap(ProposalBallot.init(rawValue:)), environment: environment
        ) { [environment, toasts, analytics, openVotes] card, outcome in
            HomeVoteCommit.accepted(card, outcome, environment: environment, toasts: toasts, analytics: analytics)
            Task { await openVotes?.reload() }
        }
        return HomeVoteCard(
            model: model, identifierPrefix: "chat-pin-list", tab: hostMainTab ?? environment.navigator.selectedTab,
            inCabal: true, canVote: summary.canVote, open: { goToVote(id) })
    }

    private func row(_ message: ChatMessage) -> some View {
        HStack(alignment: .top, spacing: MonacoTheme.Space.sm) {
            Button {
                open(message.id)
            } label: {
                content(message)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityElement(children: .combine)
            .accessibilityIdentifier("chat-pin-row-\(message.id)")
            Image(systemName: "pin")
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
                .frame(minHeight: ChatPinMetrics.minTarget)
                .accessibilityHidden(true)
            if controls.canPin(message) {
                Button(ChatPinCopy.unpin) { controls.unpin(message, toasts: toasts) }
                    .font(MonacoTheme.Typo.subheadStrong)
                    .foregroundStyle(MonacoTheme.primaryText)
                    .buttonStyle(.plain)
                    .frame(minHeight: ChatPinMetrics.minTarget)
                    .accessibilityIdentifier("chat-pin-unpin-\(message.id)")
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.vertical, MonacoTheme.Space.s)
    }

    private func content(_ message: ChatMessage) -> some View {
        HStack(alignment: .top, spacing: MonacoTheme.Space.sm) {
            MonacoAvatar(
                photoURL: message.author.photoUrl, displayName: message.author.displayName, size: 36,
                seed: message.author.id
            )
            .accessibilityHidden(true)
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                HStack(spacing: MonacoTheme.Space.s) {
                    Text(ChatPinCopy.firstName(message.author.displayName))
                        .font(MonacoTheme.Typo.headline)
                        .foregroundStyle(MonacoTheme.primaryText)
                    Text(message.createdAt.formatted(.dateTime.month(.abbreviated).day().hour().minute()))
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.secondaryText)
                }
                Text(ChatPinCopy.text(of: message, cabalName: cabalName, viewerID: viewerID))
                    .font(MonacoTheme.Typo.body)
                    .foregroundStyle(MonacoTheme.primaryText)
                    .lineLimit(ChatPinMetrics.listRowLines)
                    .multilineTextAlignment(.leading)
            }
        }
    }
}
