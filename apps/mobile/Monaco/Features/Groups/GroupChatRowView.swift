import MonacoAPI
import MonacoCore
import SwiftUI

struct GroupChatRowView: View {
    let row: ChatRow
    let now: Date
    let openProfile: (String) -> Void
    let retry: (String) -> Void
    var openThread: ((String) -> Void)?

    private var message: ChatMessage { row.message }

    var body: some View {
        VStack(alignment: .leading, spacing: 3) {
            if let label = row.dayLabel(now: now) {
                ChatDayRule(label: label)
                    .padding(.top, 16)
                    .padding(.bottom, 4)
                    .accessibilityIdentifier("chat-day-\(row.id)")
            }
            HStack(alignment: .top, spacing: 0) {
                if row.isMine { Spacer(minLength: 56) }
                VStack(alignment: row.isMine ? .trailing : .leading, spacing: 4) {
                    if row.startsRun && !row.isMine { authorHeader }
                    threadHeader
                    bubble
                        .modifier(ChatMessageMenu(enabled: canOpenMenu, reply: { openThread?(rootID) }))
                    footer
                }
                if !row.isMine { Spacer(minLength: 56) }
            }
            .padding(.top, row.startsRun && !row.startsDay ? 10 : 0)
        }
    }

    private var authorHeader: some View {
        let author = message.author
        let named = !author.displayName.isEmpty
        return Button {
            openProfile(author.id)
        } label: {
            HStack(spacing: MonacoTheme.Space.s) {
                MonacoAvatar(photoURL: author.photoUrl, displayName: author.displayName, size: 24, seed: author.id)
                if named {
                    Text(author.displayName)
                        .font(MonacoTheme.Typo.captionStrong)
                        .foregroundStyle(MonacoTheme.muted)
                        .lineLimit(1)
                }
            }
            .frame(minHeight: 28)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .disabled(!named)
        .accessibilityLabel(named ? "\(author.displayName), open profile" : "Former member")
        .accessibilityIdentifier("chat-author-\(row.id)")
    }

    private var bubble: some View {
        Text(message.deleted ? GroupChatCopy.deleted : (message.body ?? ""))
            .font(MonacoTheme.Typo.body)
            .italic(message.deleted)
            .foregroundStyle(textColor)
            .padding(.horizontal, 14)
            .padding(.vertical, 9)
            .background(bubbleShape.fill(row.isMine ? MonacoTheme.brandFill : MonacoTheme.surface))
            .overlay {
                if !row.isMine { bubbleShape.strokeBorder(MonacoTheme.hairline, lineWidth: 1) }
            }
            .opacity(row.delivery == .pending ? 0.5 : 1)
            .accessibilityElement(children: .combine)
            .accessibilityLabel(accessibilityText)
            .accessibilityIdentifier("chat-message-\(row.id)")
    }

    @ViewBuilder private var footer: some View {
        if row.delivery == .failed {
            Button {
                retry(row.id)
            } label: {
                Text(GroupChatCopy.notSent)
                    .font(MonacoTheme.Typo.captionStrong)
                    .foregroundStyle(MonacoTheme.destructive)
                    .frame(minHeight: 44)
                    .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityIdentifier("chat-retry-\(row.id)")
        } else if message.replyCount > 0 {
            repliesRow
        }
    }

    private var rootID: String { message.parentId ?? message.id }

    private var canOpenMenu: Bool {
        openThread != nil && !message.deleted && row.delivery == .sent
    }

    @ViewBuilder private var threadHeader: some View {
        if message.alsoInChannel, let parentID = message.parentId, let openThread {
            Button {
                openThread(parentID)
            } label: {
                HStack(spacing: 4) {
                    Image(systemName: "arrowshape.turn.up.left")
                    Text(ChatThreadCopy.header(parentBody: row.parentBody)).lineLimit(1)
                }
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
                .frame(minHeight: 32)
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityIdentifier("chat-thread-header-\(row.id)")
        }
    }

    @ViewBuilder private var repliesRow: some View {
        let summary = ChatThreadCopy.summary(
            replyCount: message.replyCount, lastReplyAt: message.lastReplyAt, now: now)
        if let openThread {
            Button {
                openThread(message.id)
            } label: {
                HStack(spacing: 4) {
                    Text(summary)
                    Image(systemName: "chevron.right").font(.caption2.weight(.semibold))
                }
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
                .frame(minHeight: 44)
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityIdentifier("chat-replies-\(row.id)")
        } else {
            Text(summary)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
                .accessibilityIdentifier("chat-replies-\(row.id)")
        }
    }

    private var textColor: Color {
        if message.deleted { return MonacoTheme.muted }
        return row.isMine ? MonacoTheme.onBrand : MonacoTheme.ink
    }

    private var bubbleShape: UnevenRoundedRectangle {
        let radius = MonacoTheme.Radius.bubble
        let tail: CGFloat = row.endsRun ? 6 : radius
        return UnevenRoundedRectangle(
            topLeadingRadius: radius,
            bottomLeadingRadius: row.isMine ? radius : tail,
            bottomTrailingRadius: row.isMine ? tail : radius,
            topTrailingRadius: radius,
            style: .continuous
        )
    }

    private var accessibilityText: String {
        let who = row.isMine ? "You" : message.author.displayName
        let body = message.deleted ? GroupChatCopy.deleted : (message.body ?? "")
        let time = row.date.formatted(date: .omitted, time: .shortened)
        return who.isEmpty ? "\(time): \(body)" : "\(who), \(time): \(body)"
    }
}

struct ChatDayRule: View {
    let label: String

    var body: some View {
        HStack(spacing: MonacoTheme.Space.sm) {
            MonacoRule()
            Text(label)
                .font(MonacoTheme.Typo.stamp)
                .foregroundStyle(MonacoTheme.tertiaryText)
                .multilineTextAlignment(.center)
                .lineLimit(2)
                .layoutPriority(1)
            MonacoRule()
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(label)
    }
}

struct ChatMessageMenu: ViewModifier {
    let enabled: Bool
    let reply: () -> Void

    @ViewBuilder func body(content: Content) -> some View {
        if enabled {
            content.contextMenu {
                Button {
                    reply()
                } label: {
                    Label(ChatThreadCopy.reply, systemImage: "arrowshape.turn.up.left")
                }
            }
        } else {
            content
        }
    }
}
