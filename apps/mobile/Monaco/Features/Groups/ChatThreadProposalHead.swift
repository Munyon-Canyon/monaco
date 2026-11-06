import MonacoAPI
import MonacoCore
import SwiftUI

enum ChatThreadHeadMetrics {
    static let faceSize: CGFloat = 40
    static let quoteBar: CGFloat = 2
}

struct ChatThreadProposalHead: View {
    let parent: ChatMessage
    let proposalID: String
    let store: ChatProposalStore
    let members: [Components.Schemas.CabalMember]
    let now: Date
    let openProfile: (String) -> Void
    let openProposal: (String) -> Void

    static func thesis(of parent: ChatMessage) -> String? {
        guard let body = parent.body?.trimmingCharacters(in: .whitespacesAndNewlines), !body.isEmpty else {
            return nil
        }
        return body
    }

    static func ageText(of parent: ChatMessage, now: Date) -> String {
        RelativeTimeFormatter.label(date: parent.createdAt, now: now)
    }

    static func showsRule(of parent: ChatMessage) -> Bool { parent.replyCount > 0 }

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            authorRow
            if let thesis = Self.thesis(of: parent) {
                ChatMessageText(
                    text: thesis, members: members, color: MonacoTheme.bodyText,
                    mentionColor: MonacoTheme.brand, openProfile: openProfile
                )
                .fixedSize(horizontal: false, vertical: true)
                .accessibilityIdentifier("chat-thread-parent-body")
            }
            ChatProposalCard(
                message: parent, proposalID: proposalID, store: store, columnInset: 0, isMine: false,
                openProposal: { openProposal(proposalID) },
                name: { EmptyView() }, thesis: { EmptyView() }, face: { EmptyView() }, replies: { EmptyView() })
            if Self.showsRule(of: parent) { repliesRule }
        }
        .padding(.top, MonacoTheme.Space.s)
        .padding(.bottom, MonacoTheme.Space.s)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("chat-thread-proposal-head")
    }

    private var authorRow: some View {
        Button {
            openProfile(parent.author.id)
        } label: {
            HStack(spacing: MonacoTheme.Space.s) {
                MonacoAvatar(
                    photoURL: parent.author.photoUrl, displayName: parent.author.displayName,
                    size: ChatThreadHeadMetrics.faceSize, seed: parent.author.id)
                Text(parent.author.displayName)
                    .font(MonacoTheme.Typo.headline)
                    .foregroundStyle(MonacoTheme.primaryText)
                    .lineLimit(1)
                    .monacoPrivate()
                Text(Self.ageText(of: parent, now: now))
                    .font(MonacoTheme.Typo.subhead)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .accessibilityIdentifier("chat-thread-parent-time")
                Spacer(minLength: 0)
            }
            .frame(minHeight: MonacoButtonMetrics.hitTarget)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .disabled(parent.author.displayName.isEmpty)
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("chat-thread-parent-author")
    }

    private var repliesRule: some View {
        HStack(spacing: MonacoTheme.Space.s) {
            MonacoRule()
            Text(GroupChatCopy.replies(parent.replyCount))
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.secondaryText)
                .fixedSize()
            MonacoRule()
        }
        .padding(.top, MonacoTheme.Space.s)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(GroupChatCopy.replies(parent.replyCount))
        .accessibilityAddTraits(.isHeader)
        .accessibilityIdentifier("chat-thread-replies-rule")
    }
}

struct ChatReplyQuote: View {
    struct Display: Equatable {
        let name: String
        let excerpt: String
        let isDeleted: Bool
        let targetID: String

        init(_ quote: Components.Schemas.CommentQuote) {
            name = quote.authorDisplayName
            targetID = quote.commentId
            isDeleted = quote.isDeleted
            if quote.isDeleted {
                excerpt = ChatThreadCopy.quoteDeleted
            } else if let text = quote.excerpt, !text.isEmpty {
                excerpt = text
            } else if quote.hasAttachment {
                excerpt = "GIF"
            } else {
                excerpt = ChatThreadCopy.quoteDeleted
            }
        }

        var canJump: Bool { !isDeleted }
    }

    let quote: Components.Schemas.CommentQuote
    let rowID: String
    let jump: ((String) -> Void)?

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    private var display: Display { Display(quote) }

    var body: some View {
        if display.canJump, let jump {
            Button {
                jump(display.targetID)
            } label: {
                line
            }
            .buttonStyle(.plain)
            .accessibilityAddTraits(.isButton)
            .accessibilityIdentifier("chat-reply-quote-\(rowID)")
        } else {
            line.accessibilityIdentifier("chat-reply-quote-\(rowID)")
        }
    }

    private var line: some View {
        HStack(spacing: MonacoTheme.Space.s) {
            Rectangle()
                .fill(MonacoTheme.tertiaryText)
                .frame(width: ChatThreadHeadMetrics.quoteBar)
            text
                .lineLimit(dynamicTypeSize.isAccessibilitySize ? nil : 1)
                .truncationMode(.tail)
                .monacoPrivate()
        }
        .fixedSize(horizontal: false, vertical: true)
        .frame(maxWidth: .infinity, alignment: .leading)
        .frame(minHeight: MonacoButtonMetrics.hitTarget)
        .contentShape(Rectangle())
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(ChatThreadCopy.quoteSpoken(name: display.name, excerpt: display.excerpt))
    }

    private var text: Text {
        if display.isDeleted {
            return Text(display.excerpt)
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.tertiaryText)
        }
        return Text(display.name)
            .font(Font.footnote.weight(.semibold))
            .foregroundStyle(MonacoTheme.primaryText)
            + Text(" ")
            + Text(display.excerpt)
            .font(MonacoTheme.Typo.subhead)
            .foregroundStyle(MonacoTheme.secondaryText)
    }
}
