import MonacoAPI
import MonacoCore
import SwiftUI

enum ChatProposalMetrics {
    static let avatarSize: CGFloat = 38
    static let footerHeight: CGFloat = 44
}

struct ChatProposalCard: View {
    let message: ChatMessage
    let proposalID: String
    let store: ChatProposalStore
    let members: [Components.Schemas.CabalMember]
    let cabalName: String
    let now: Date
    let openProposal: () -> Void
    let openThread: (() -> Void)?

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    private var state: ChatProposalState { store.state(for: proposalID) }

    var body: some View {
        let shape = RoundedRectangle(cornerRadius: MonacoTheme.Radius.card, style: .continuous)
        VStack(spacing: 0) {
            mainRow
                .padding(.horizontal, MonacoTheme.Space.sm)
                .padding(.top, MonacoTheme.Space.sm)
                .padding(.bottom, MonacoTheme.Space.xs)
            MonacoRule()
            footer
        }
        .background(shape.fill(MonacoTheme.surface))
        .overlay(shape.strokeBorder(MonacoTheme.hairline, lineWidth: 1))
        .padding(.horizontal, MonacoTheme.Space.sm)
        .padding(.vertical, MonacoTheme.Space.l)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("chat-proposal-\(message.id)")
        .task(id: proposalID) { await store.load(proposalID) }
    }

    private var proposer: (name: String, id: String, photoURL: String?) {
        if case .loaded(let facts) = state,
            let member = members.first(where: { $0.userId == facts.proposerID })
        {
            return (member.displayName, member.userId, member.photoUrl)
        }
        let author = message.author
        return (author.displayName, author.id, author.photoUrl)
    }

    private var layout: AnyLayout {
        dynamicTypeSize.isAccessibilitySize
            ? AnyLayout(VStackLayout(alignment: .leading, spacing: MonacoTheme.Space.s))
            : AnyLayout(HStackLayout(alignment: .center, spacing: MonacoTheme.Space.s))
    }

    @ViewBuilder private var mainRow: some View {
        switch state {
        case .loading:
            HStack(spacing: MonacoTheme.Space.s) {
                SkeletonBlock(
                    width: ChatProposalMetrics.avatarSize, height: ChatProposalMetrics.avatarSize,
                    radius: ChatProposalMetrics.avatarSize / 2)
                VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                    SkeletonBar(width: 180)
                    SkeletonBar(width: 120)
                }
                Spacer(minLength: 0)
            }
            .frame(minHeight: MonacoButtonMetrics.hitTarget)
            .accessibilityElement(children: .ignore)
            .accessibilityLabel("Loading proposal")
            .accessibilityIdentifier("chat-proposal-loading-\(message.id)")
        case .loaded(let facts):
            loadedRow(
                title: facts.title(proposer: proposer.name), amount: facts.amountLine, buttonTitle: facts.buttonTitle)
        case .failed:
            loadedRow(title: GroupChatCopy.unknownProposal, amount: nil, buttonTitle: GroupChatCopy.review)
        }
    }

    private func loadedRow(title: String, amount: String?, buttonTitle: String) -> some View {
        layout {
            HStack(spacing: MonacoTheme.Space.s) {
                MonacoAvatar(
                    photoURL: proposer.photoURL, displayName: proposer.name, size: ChatProposalMetrics.avatarSize,
                    seed: proposer.id)
                texts(title: title, amount: amount)
            }
            if !dynamicTypeSize.isAccessibilitySize { Spacer(minLength: MonacoTheme.Space.xs) }
            Button(buttonTitle, action: openProposal)
                .buttonStyle(.monacoCompactProminent)
                .accessibilityIdentifier("chat-proposal-review-\(message.id)")
        }
    }

    private func texts(title: String, amount: String?) -> some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
            Text(title)
                .font(MonacoTheme.Typo.subheadStrong)
                .foregroundStyle(MonacoTheme.primaryText)
                .fixedSize(horizontal: false, vertical: true)
            if let amount {
                Text(amount)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.secondaryText)
            }
            Text(GroupChatCopy.statusLine(time: cabalName, status: age))
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityElement(children: .combine)
    }

    private var age: String {
        Self.relative.localizedString(for: message.createdAt, relativeTo: now)
    }

    private static let relative: RelativeDateTimeFormatter = {
        let formatter = RelativeDateTimeFormatter()
        formatter.unitsStyle = .short
        formatter.dateTimeStyle = .named
        return formatter
    }()

    @ViewBuilder private var footer: some View {
        let isApproved = { if case .loaded(let facts) = state { facts.phase == .approved } else { false } }()
        let label = isApproved ? GroupChatCopy.executionPending : repliesText
        let content = HStack(spacing: MonacoTheme.Space.s) {
            if isApproved {
                Image(systemName: "clock")
                    .foregroundStyle(MonacoTheme.warning)
                    .accessibilityHidden(true)
            }
            Text(label)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
            Spacer(minLength: 0)
            Image(systemName: "chevron.right")
                .font(MonacoTheme.Typo.captionStrong)
                .foregroundStyle(MonacoTheme.secondaryText)
                .accessibilityHidden(true)
        }
        .padding(.horizontal, MonacoTheme.Space.sm)
        .frame(maxWidth: .infinity, minHeight: ChatProposalMetrics.footerHeight, alignment: .leading)
        .contentShape(Rectangle())
        if let openThread {
            Button(action: openThread) { content }
                .buttonStyle(.plain)
                .accessibilityIdentifier("chat-replies-\(message.id)")
        } else {
            content
        }
    }

    private var repliesText: String {
        message.replyCount > 0 ? GroupChatCopy.replies(message.replyCount) : GroupChatCopy.discuss
    }
}
