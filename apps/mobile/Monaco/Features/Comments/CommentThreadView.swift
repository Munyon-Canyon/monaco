import MonacoAPI
import MonacoCore
import SwiftUI
import UIKit

struct CommentThreadView: View {
    let model: CommentsModel
    @State private var deleting: CommentThreadRow?
    @State private var reporting: ReportTarget?

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            if model.phase != .empty {
                MonacoSectionHeader(CommentsCopy.title)
                    .padding(.horizontal, MonacoTheme.Space.gutter)
            }
            content
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("comment-thread")
        .commentNotices(model)
        .reportSheet($reporting)
        .onReceive(NotificationCenter.default.publisher(for: .monacoBlocksChanged)) { _ in
            Task { await model.refresh() }
        }
        .confirmationDialog(
            "Delete this comment?",
            isPresented: Binding(get: { deleting != nil }, set: { if !$0 { deleting = nil } }),
            titleVisibility: .visible,
            presenting: deleting
        ) { row in
            Button(CommentsCopy.delete, role: .destructive) { Task { await model.delete(row) } }
            Button("Cancel", role: .cancel) {}
        }
    }

    @ViewBuilder private var content: some View {
        switch model.phase {
        case .loading:
            CommentSkeleton()
                .accessibilityElement(children: .ignore)
                .accessibilityLabel("Loading comments")
                .accessibilityIdentifier("comment-thread-loading")
        case .empty:
            EmptyState(
                title: CommentsCopy.empty, message: CommentsCopy.emptyMessage, systemImage: MonacoIcon.chat.symbol
            )
            .accessibilityIdentifier("comment-thread-empty")
        case .failed:
            MonacoErrorRow(thing: "comments", identifier: "comment-thread-error") {
                Task { await model.load() }
            }
        case .loaded:
            loaded
        }
    }

    private var loaded: some View {
        LazyVStack(alignment: .leading, spacing: 0) {
            ForEach(model.rows) { row in
                CommentRow(
                    row: row,
                    onReply: { model.beginReply(to: row) },
                    onDelete: row.comment.isMine && !row.isDeleted ? { deleting = row } : nil,
                    onReport: !row.comment.isMine && !row.isDeleted
                        ? { reporting = ReportTarget(kind: .comment, targetID: row.id) } : nil
                )
                .id(row.id)
            }
            if model.isLoadingMore {
                CommentSkeleton(rows: 1)
            }
            if model.hasMore {
                Color.clear.frame(height: 1)
                    .onAppear { Task { await model.loadMore() } }
            }
        }
        .frame(maxWidth: .infinity)
    }
}

private struct CommentSkeleton: View {
    var rows = 3

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.l) {
            ForEach(0..<rows, id: \.self) { _ in
                HStack(alignment: .top, spacing: MonacoTheme.Space.sm) {
                    SkeletonBlock(
                        width: CommentThreadLayout.avatarSize, height: CommentThreadLayout.avatarSize,
                        radius: CommentThreadLayout.avatarSize / 2)
                    VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                        SkeletonFraction(SkeletonMetrics.rowSubtitleFraction) { SkeletonBar() }
                        SkeletonFraction(SkeletonMetrics.rowTitleFraction) { SkeletonBar() }
                    }
                }
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.vertical, MonacoTheme.Space.s)
    }
}

enum CommentThreadLayout {
    static let avatarSize: CGFloat = MonacoAvatarSize.comment.rawValue
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

struct CommentRow: View {
    let row: CommentThreadRow
    let onReply: () -> Void
    var onDelete: (() -> Void)?
    var onReport: (() -> Void)?

    @Environment(\.feedOpensSoon) private var opensSoon
    @ScaledMetric(relativeTo: .callout) private var nameLineHeight: CGFloat = 20

    private var level: Int { row.indentLevel }

    private var authorReturn: FeedReturnFacts? {
        FeedReturnFacts(opensSoon.commentReturns[row.authorID])
    }

    private var time: String {
        RelativeTimeFormatter.label(date: row.comment.createdAt, now: Date())
    }

    var body: some View {
        HStack(alignment: .top, spacing: MonacoTheme.Space.sm) {
            authorLink(
                identifier: "comment-avatar-\(row.id)", height: CommentThreadLayout.avatarSize,
                widthOverhang: CommentRowMetrics.replyTarget - CommentThreadLayout.avatarSize
            ) {
                MonacoAvatar(
                    photoURL: row.comment.author.photoUrl, displayName: row.authorName,
                    size: CommentThreadLayout.avatarSize, seed: row.authorID)
            }
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                byline
                Text(bodyText)
                    .font(MonacoTheme.Typo.subhead)
                    .foregroundStyle(row.isDeleted ? MonacoTheme.secondaryText : MonacoTheme.bodyText)
                    .fixedSize(horizontal: false, vertical: true)
                    .accessibilityIdentifier("comment-body-\(row.id)")
                if !row.isDeleted { replyButton }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .padding(.leading, CommentThreadLayout.avatarLeading(level: level))
        .padding(.trailing, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.s)
        .frame(maxWidth: .infinity, alignment: .leading)
        .contentShape(Rectangle())
        .contextMenu {
            Button("Copy") { UIPasteboard.general.string = row.text }
                .accessibilityIdentifier("comment-copy-\(row.id)")
            if let onReport {
                Button(action: onReport) {
                    Label("Report", systemImage: "exclamationmark.bubble")
                }
                .accessibilityIdentifier("comment-report-\(row.id)")
            }
            if let onDelete {
                Button(CommentsCopy.delete, role: .destructive, action: onDelete)
                    .accessibilityIdentifier("comment-delete-\(row.id)")
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("comment-row-\(row.id)")
    }

    private var byline: some View {
        ViewThatFits(in: .horizontal) {
            HStack(alignment: .firstTextBaseline, spacing: MonacoTheme.Space.s) {
                nameLink
                authorReturnPill
                timeLabel
            }
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                nameLink
                authorReturnPill
                timeLabel
            }
        }
    }

    private var nameLink: some View {
        authorLink(identifier: "comment-author-\(row.id)", height: nameLineHeight) {
            Text(row.authorName)
                .font(MonacoTheme.Typo.subheadStrong)
                .foregroundStyle(MonacoTheme.primaryText)
                .lineLimit(1)
        }
    }

    @ViewBuilder private var authorReturnPill: some View {
        if let authorReturn { FeedReturnPill(facts: authorReturn) }
    }

    private var timeLabel: some View {
        Text(time)
            .font(MonacoTheme.Typo.caption)
            .foregroundStyle(MonacoTheme.secondaryText)
            .fixedSize()
    }

    private var bodyText: AttributedString {
        var text = AttributedString(row.text)
        if let handle = row.replyToHandle {
            var mention = AttributedString("@\(handle) ")
            mention.foregroundColor = MonacoTheme.secondaryText
            text = mention + text
        }
        return text
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
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.brand)
                .frame(
                    minWidth: CommentRowMetrics.replyTarget, minHeight: CommentRowMetrics.replyTarget,
                    alignment: .leading
                )
                .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .padding(.top, MonacoTheme.Space.xs)
        .accessibilityLabel(CommentsCopy.reply)
        .accessibilityIdentifier("comment-reply-\(row.id)")
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
