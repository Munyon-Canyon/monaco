import MonacoAPI
import MonacoCore
import MonacoNotify
import SwiftUI

struct CatchUpCardComment: View {
    let item: Components.Schemas.CatchUpItem
    let comment: Components.Schemas.CatchUpComment
    let now: Date
    let reactions: ReactionsModel
    let onReply: () -> Void

    @Environment(\.calendar) private var calendar
    @State private var settled: [Components.Schemas.Reaction]?

    private static let tap: CGFloat = 44

    var body: some View {
        HStack(alignment: .top, spacing: MonacoTheme.Space.sm) {
            MonacoAvatar(
                photoURL: comment.author.photoUrl, displayName: comment.author.name, size: 32,
                seed: comment.author.id)
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                if item.kind == .reply, let quote = CatchUpCopy.quote(item) {
                    quoteLine(quote)
                }
                HStack(alignment: .firstTextBaseline, spacing: MonacoTheme.Space.s) {
                    Text(comment.author.name)
                        .font(MonacoTheme.Typo.headline)
                        .foregroundStyle(MonacoTheme.primaryText)
                    Text(CatchUpCopy.relativeTime(comment.createdAt, now: now, calendar: calendar))
                        .font(MonacoTheme.Typo.subhead)
                        .foregroundStyle(MonacoTheme.secondaryText)
                }
                body(comment.body)
                    .accessibilityElement(children: .ignore)
                    .accessibilityLabel(comment.body)
                reactionRow
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .monacoPrivate()
        }
        .padding(MonacoTheme.Space.sm)
        .background(MonacoTheme.canvas, in: RoundedRectangle(cornerRadius: MonacoTheme.Radius.card))
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("catch-up-deck-comment")
    }

    private func quoteLine(_ quote: (author: String, text: String)) -> some View {
        HStack(spacing: MonacoTheme.Space.s) {
            Image(monaco: .replyArrow)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.brand)
            (Text(quote.author).font(MonacoTheme.Typo.subheadStrong).foregroundStyle(MonacoTheme.primaryText)
                + Text(" ") + Text(quote.text).font(MonacoTheme.Typo.subhead).foregroundStyle(MonacoTheme.secondaryText))
                .lineLimit(1)
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("Replying to \(quote.author): \(quote.text)")
        .accessibilityIdentifier("catch-up-deck-quote")
    }

    private func body(_ text: String) -> some View {
        CatchUpDeckCopy.segments(text).reduce(Text(verbatim: "")) { line, segment in
            let color: Color = segment.isMention ? MonacoTheme.tint : MonacoTheme.bodyText
            return line + Text(verbatim: segment.text).foregroundStyle(color)
        }
        .font(MonacoTheme.Typo.body)
        .fixedSize(horizontal: false, vertical: true)
    }

    private var shown: [Components.Schemas.Reaction] {
        reactions.reactions(for: target, fallback: settled ?? comment.reactions)
    }

    private var target: ReactionTarget { .comment(comment.commentId) }

    private var reactionRow: some View {
        ReactionRow(
            target: target, reactions: shown, showsAdd: false, size: .compact,
            onToggle: { emoji in toggle(emoji) },
            trailing: {
                Button(action: onReply) {
                    Text(CatchUpDeckCopy.reply)
                        .font(MonacoTheme.Typo.subhead)
                        .foregroundStyle(MonacoTheme.brand)
                        .frame(minWidth: Self.tap, minHeight: Self.tap, alignment: .leading)
                        .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .accessibilityLabel(CatchUpDeckCopy.reply)
                .accessibilityIdentifier("catch-up-deck-comment-reply")
            })
    }

    private func toggle(_ emoji: String) {
        let current = shown
        let model = reactions
        let target = target
        Task {
            settled = await model.toggle(emoji, on: target, current: current)
        }
    }
}
