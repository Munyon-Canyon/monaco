import MonacoAPI
import MonacoCore
import SwiftUI
import UIKit

enum CommentThreadLayout {
    static let avatarSize: CGFloat = MonacoAvatarSize.feed.rawValue
    static let indentWidth: CGFloat = avatarSize + MonacoTheme.Space.sm

    static func avatarLeading(level: Int) -> CGFloat {
        MonacoTheme.Space.gutter + CGFloat(max(level, 0)) * indentWidth
    }

    static func textLeading(level: Int) -> CGFloat {
        avatarLeading(level: level) + avatarSize + MonacoTheme.Space.sm
    }
}

enum CommentRowMetrics {
    static let replyTarget: CGFloat = 44

    static func replyOverhang(lineHeight: CGFloat) -> CGFloat {
        max(0, (replyTarget - lineHeight) / 2)
    }

    static func nameSlop(overhang: CGFloat) -> CGFloat {
        overhang > 0 ? 0 : MonacoTheme.Space.s
    }
}

private final class RowFrameBox {
    var frame: CGRect = .zero
}

struct CommentRow: View {
    let row: CommentThreadRow
    var reactions: [Components.Schemas.Reaction]?
    var isHighlighted = false
    var canReply = true
    let onReply: () -> Void
    var onToggleReaction: (String) -> Void = { _ in }
    var onDoubleTap: () -> Void = {}
    var onJump: (CommentQuoteRow) -> Void = { _ in }
    var onDelete: (() -> Void)?
    var onReport: (() -> Void)?
    var onBlock: (() -> Void)?
    var onLongPress: ((CGRect) -> Void)?

    @State private var frameBox = RowFrameBox()
    @ScaledMetric(relativeTo: .callout) private var nameLineHeight: CGFloat = 20

    private var canLongPress: Bool { !row.isDeleted && !row.isSystem }

    private var level: Int { row.indentLevel }

    private var shownReactions: [Components.Schemas.Reaction] { reactions ?? row.reactions }

    private var time: String {
        if row.isSystem {
            return Self.systemTime.string(from: row.comment.createdAt)
        }
        return RelativeTimeFormatter.label(date: row.comment.createdAt, now: Date())
    }

    private static let systemTime: DateFormatter = {
        let formatter = DateFormatter()
        formatter.setLocalizedDateFormatFromTemplate("MMM d jm")
        return formatter
    }()

    private var showsBody: Bool { row.isDeleted || row.comment.body != nil }

    var body: some View {
        HStack(alignment: .top, spacing: MonacoTheme.Space.sm) {
            avatar
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                byline
                if let quote = row.quote {
                    CommentQuote(quote: quote, rowID: row.id) { onJump(quote) }
                }
                if showsBody { bodyText }
                if let attachment = row.attachment { AttachmentBubble(commentID: row.id, attachment: attachment) }
                if row.canReact { reactionRow }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .padding(.leading, CommentThreadLayout.avatarLeading(level: level))
        .padding(.trailing, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.s)
        .padding(.bottom, row.canReact ? 0 : MonacoTheme.Space.m)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(isHighlighted ? MonacoTheme.surfaceSunken : Color.clear)
        .animation(.easeOut(duration: 0.2), value: isHighlighted)
        .frame(minHeight: CommentRowMetrics.replyTarget, alignment: .top)
        .contentShape(Rectangle())
        .onGeometryChange(for: CGRect.self) {
            $0.frame(in: .global)
        } action: {
            frameBox.frame = $0
        }
        .onTapGesture(count: 2) { if row.canReact { onDoubleTap() } }
        .onLongPressGesture {
            guard canLongPress, let onLongPress else { return }
            Haptics.tap()
            onLongPress(frameBox.frame)
        }
        .accessibilityElement(children: .contain)
        .accessibilityLabel(row.spoken(age: time))
        .accessibilityIdentifier("comment-row-\(row.id)")
        .modifier(
            CommentActions(
                row: row, canReply: canReply, onReply: onReply, onReact: onDoubleTap, onJump: onJump,
                onReport: onReport, onBlock: onBlock, onDelete: onDelete))
    }

    @ViewBuilder private var avatar: some View {
        if row.isSystem {
            Circle()
                .fill(MonacoTheme.surfaceSunken)
                .frame(width: CommentThreadLayout.avatarSize, height: CommentThreadLayout.avatarSize)
                .overlay {
                    Image(monaco: .bot).font(MonacoTheme.Typo.subhead).foregroundStyle(MonacoTheme.secondaryText)
                }
                .accessibilityHidden(true)
        } else {
            authorLink(
                identifier: "comment-avatar-\(row.id)", height: CommentThreadLayout.avatarSize,
                widthOverhang: CommentRowMetrics.replyTarget - CommentThreadLayout.avatarSize
            ) {
                MonacoAvatar(
                    photoURL: row.comment.author.photoUrl, displayName: row.authorName,
                    size: CommentThreadLayout.avatarSize, seed: row.authorID)
            }
            .accessibilityHidden(true)
        }
    }

    private var byline: some View {
        ViewThatFits(in: .horizontal) {
            HStack(alignment: .firstTextBaseline, spacing: MonacoTheme.Space.s) {
                nameLink
                timeLabel
            }
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                nameLink
                timeLabel
            }
        }
    }

    private var nameLink: some View {
        authorLink(identifier: "comment-author-\(row.id)", height: nameLineHeight) {
            Text(row.authorName)
                .font(MonacoTheme.Typo.headline)
                .foregroundStyle(MonacoTheme.primaryText)
                .fixedSize(horizontal: false, vertical: true)
                .monacoPrivate()
        }
    }

    private var timeLabel: some View {
        Text(time)
            .font(MonacoTheme.Typo.subhead)
            .foregroundStyle(MonacoTheme.secondaryText)
            .fixedSize()
            .accessibilityHidden(true)
    }

    private var bodyText: some View {
        Text(row.text)
            .font(MonacoTheme.Typo.body)
            .foregroundStyle(row.isDeleted ? MonacoTheme.secondaryText : MonacoTheme.bodyText)
            .fixedSize(horizontal: false, vertical: true)
            .accessibilityIdentifier("comment-body-\(row.id)")
            .monacoPrivate()
    }

    private var reactionRow: some View {
        ReactionRow(
            target: .comment(row.id), reactions: shownReactions, showsAdd: false, size: .compact,
            onToggle: onToggleReaction
        ) {
            if canReply { replyButton }
        }
    }

    @ViewBuilder private func authorLink(
        identifier: String, height: CGFloat, widthOverhang: CGFloat = 0, @ViewBuilder label: () -> some View
    ) -> some View {
        if row.opensAuthorProfile {
            NavigationLink(value: AnyAppRoute(UserProfileRoute(userID: row.authorID))) {
                label()
                    .padding(.horizontal, CommentRowMetrics.nameSlop(overhang: widthOverhang))
                    .frame(
                        minWidth: widthOverhang > 0 ? CommentRowMetrics.replyTarget : nil,
                        minHeight: CommentRowMetrics.replyTarget, alignment: .leading
                    )
                    .contentShape(Rectangle())
                    .padding(.horizontal, -CommentRowMetrics.nameSlop(overhang: widthOverhang))
            }
            .buttonStyle(.plain)
            .padding(.vertical, -CommentRowMetrics.replyOverhang(lineHeight: height))
            .padding(.trailing, -widthOverhang)
            .accessibilityIdentifier(identifier)
        } else {
            label()
        }
    }

    private var replyButton: some View {
        Button(action: onReply) {
            Text(CommentsCopy.reply)
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.brand)
                .frame(
                    minWidth: CommentRowMetrics.replyTarget, minHeight: CommentRowMetrics.replyTarget,
                    alignment: .leading
                )
                .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityLabel(CommentsCopy.reply)
        .accessibilityIdentifier("comment-reply-\(row.id)")
    }
}

private struct CommentActions: ViewModifier {
    let row: CommentThreadRow
    let canReply: Bool
    let onReply: () -> Void
    let onReact: () -> Void
    let onJump: (CommentQuoteRow) -> Void
    let onReport: (() -> Void)?
    let onBlock: (() -> Void)?
    let onDelete: (() -> Void)?

    func body(content: Content) -> some View {
        content
            .accessibilityAction(named: CommentsCopy.reply) { if canReply && row.canReact { onReply() } }
            .accessibilityAction(named: CommentsCopy.react) { if row.canReact { onReact() } }
            .accessibilityAction(named: CommentsCopy.goToQuote) {
                if let quote = row.quote, quote.canJump { onJump(quote) }
            }
            .accessibilityAction(named: "Copy") { if !row.isSystem { UIPasteboard.general.string = row.text } }
            .accessibilityAction(named: "Report") { onReport?() }
            .accessibilityAction(named: "Block") { onBlock?() }
            .accessibilityAction(named: CommentsCopy.delete) { onDelete?() }
    }
}
