import MonacoAPI
import MonacoCore
import SwiftUI

struct CommentThreadView: View {
    let model: CommentsModel

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            Text(CommentsCopy.title)
                .font(MonacoTheme.Typo.section)
                .foregroundStyle(MonacoTheme.ink)
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .accessibilityAddTraits(.isHeader)
            content
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("comment-thread")
        .commentNotices(model)
    }

    @ViewBuilder private var content: some View {
        switch model.phase {
        case .loading:
            MonacoRowSkeleton(rows: 3, markShape: .circle)
                .accessibilityElement(children: .ignore)
                .accessibilityLabel("Loading comments")
                .accessibilityIdentifier("comment-thread-loading")
        case .empty:
            EmptyState(title: CommentsCopy.empty)
                .accessibilityIdentifier("comment-thread-empty")
        case .failed:
            HStack(spacing: MonacoTheme.Space.sm) {
                Text(CommentsCopy.loadFailed)
                    .font(MonacoTheme.Typo.body)
                    .foregroundStyle(MonacoTheme.muted)
                    .fixedSize(horizontal: false, vertical: true)
                Spacer(minLength: MonacoTheme.Space.s)
                Button(CommentsCopy.tryAgain) { Task { await model.load() } }
                    .buttonStyle(.monacoSecondary)
                    .accessibilityIdentifier("comment-thread-retry")
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .accessibilityIdentifier("comment-thread-error")
        case .loaded:
            loaded
        }
    }

    private var loaded: some View {
        let rows = model.rows
        return MonacoGroupedList {
            ForEach(Array(rows.enumerated()), id: \.element.id) { index, row in
                CommentRow(
                    row: row,
                    hasReplies: CommentThreadLayout.hasReplies(rows, at: index),
                    isLast: index == rows.count - 1,
                    onReply: { model.beginReply(to: row) },
                    onDelete: row.comment.isMine && !row.isDeleted ? { Task { await model.delete(row) } } : nil
                )
                .onAppear {
                    guard index == rows.count - 1, model.hasMore else { return }
                    Task { await model.loadMore() }
                }
            }
            if model.isLoadingMore {
                MonacoRowSkeleton(rows: 1, markShape: .circle)
            }
        }
    }
}

enum CommentThreadLayout {
    static let avatarSize: CGFloat = 32
    static let indentWidth: CGFloat = 28

    static func avatarLeading(level: Int) -> CGFloat {
        MonacoTheme.Space.m + CGFloat(max(level, 0)) * indentWidth
    }

    static func textLeading(level: Int) -> CGFloat {
        avatarLeading(level: level) + avatarSize + MonacoTheme.Space.sm
    }

    static func threadRuleOffsets(level: Int) -> [CGFloat] {
        (0..<max(level, 0)).map { avatarLeading(level: $0) + avatarSize / 2 }
    }

    static func hasReplies(_ rows: [CommentThreadRow], at index: Int) -> Bool {
        let next = index + 1
        guard rows.indices.contains(index), rows.indices.contains(next) else { return false }
        return rows[next].indentLevel > rows[index].indentLevel
    }
}

enum CommentRowMetrics {
    static let replyTarget: CGFloat = 44

    static func replyOverhang(lineHeight: CGFloat) -> CGFloat {
        max(0, (replyTarget - lineHeight) / 2)
    }
}

struct CommentRow: View {
    let row: CommentThreadRow
    var hasReplies = false
    var isLast = false
    let onReply: () -> Void
    var onDelete: (() -> Void)?

    @ScaledMetric(relativeTo: .footnote) private var replyLineHeight: CGFloat = 18

    private var level: Int { row.indentLevel }

    var body: some View {
        HStack(alignment: .top, spacing: MonacoTheme.Space.sm) {
            authorLink(identifier: "comment-avatar-\(row.id)") {
                MonacoAvatar(
                    photoURL: row.comment.author.photoUrl, displayName: row.authorName,
                    size: CommentThreadLayout.avatarSize, seed: row.authorID)
            }
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                HStack(alignment: .firstTextBaseline, spacing: MonacoTheme.Space.s) {
                    authorLink(identifier: "comment-author-\(row.id)") {
                        Text(row.authorName)
                            .font(MonacoTheme.Typo.calloutStrong)
                            .foregroundStyle(MonacoTheme.ink)
                            .lineLimit(1)
                    }
                    Text(row.comment.createdAt, format: .relative(presentation: .named, unitsStyle: .abbreviated))
                        .font(MonacoTheme.Typo.stamp)
                        .foregroundStyle(MonacoTheme.tertiaryText)
                        .fixedSize()
                }
                Text(bodyText)
                    .font(MonacoTheme.Typo.body)
                    .foregroundStyle(row.isDeleted ? MonacoTheme.muted : MonacoTheme.ink)
                    .fixedSize(horizontal: false, vertical: true)
                    .textSelection(.enabled)
                    .accessibilityIdentifier("comment-body-\(row.id)")
                if !row.isDeleted { replyButton }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .padding(.leading, CommentThreadLayout.avatarLeading(level: level))
        .padding(.trailing, MonacoTheme.Space.m)
        .padding(.top, MonacoTheme.Space.sm)
        .padding(.bottom, MonacoTheme.Space.sm)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(alignment: .topLeading) { threadRules }
        .overlay(alignment: .bottom) {
            if !isLast {
                MonacoRule().padding(.leading, CommentThreadLayout.textLeading(level: level))
            }
        }
        .contentShape(Rectangle())
        .contextMenu {
            if let onDelete {
                Button(CommentsCopy.delete, role: .destructive, action: onDelete)
                    .accessibilityIdentifier("comment-delete-\(row.id)")
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("comment-row-\(row.id)")
    }

    private var bodyText: AttributedString {
        var text = AttributedString(row.text)
        if let handle = row.replyToHandle {
            var mention = AttributedString("@\(handle) ")
            mention.foregroundColor = MonacoTheme.muted
            text = mention + text
        }
        return text
    }

    @ViewBuilder private func authorLink(identifier: String, @ViewBuilder label: () -> some View) -> some View {
        if row.opensAuthorProfile {
            NavigationLink(value: AnyAppRoute(UserProfileRoute(userID: row.authorID))) { label() }
                .buttonStyle(.plain)
                .accessibilityIdentifier(identifier)
        } else {
            label()
        }
    }

    private var replyButton: some View {
        Button(action: onReply) {
            Text(CommentsCopy.reply)
                .font(MonacoTheme.Typo.captionStrong)
                .foregroundStyle(MonacoTheme.brand)
                .frame(
                    minWidth: CommentRowMetrics.replyTarget, minHeight: CommentRowMetrics.replyTarget,
                    alignment: .leading
                )
                .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .padding(.vertical, -CommentRowMetrics.replyOverhang(lineHeight: replyLineHeight))
        .padding(.top, MonacoTheme.Space.xs)
        .accessibilityLabel(CommentsCopy.reply)
        .accessibilityIdentifier("comment-reply-\(row.id)")
    }

    private var threadRules: some View {
        ZStack(alignment: .topLeading) {
            ForEach(CommentThreadLayout.threadRuleOffsets(level: level), id: \.self) { x in
                MonacoTheme.hairline
                    .frame(width: 1)
                    .frame(maxHeight: .infinity)
                    .offset(x: x - 0.5)
            }
            if hasReplies {
                MonacoTheme.hairline
                    .frame(width: 1)
                    .frame(maxHeight: .infinity)
                    .padding(.top, MonacoTheme.Space.sm + CommentThreadLayout.avatarSize + MonacoTheme.Space.xs)
                    .offset(
                        x: CommentThreadLayout.avatarLeading(level: level) + CommentThreadLayout.avatarSize / 2 - 0.5)
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        .accessibilityHidden(true)
    }
}

extension View {
    func commentNotices(_ model: CommentsModel) -> some View {
        modifier(CommentNotices(model: model))
    }
}

private struct CommentNotices: ViewModifier {
    let model: CommentsModel
    @Environment(ToastCenter.self) private var toasts

    func body(content: Content) -> some View {
        content.onChange(of: model.noticeTick) { _, _ in
            switch model.notice {
            case .failure(let error): toasts.show(error)
            case .deleted: toasts.show(success: CommentsCopy.deleted)
            case nil: break
            }
        }
    }
}
