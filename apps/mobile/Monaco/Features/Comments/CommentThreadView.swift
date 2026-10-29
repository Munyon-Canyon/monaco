import MonacoAPI
import MonacoCore
import SwiftUI
import UIKit

struct CommentThreadView: View {
    let model: CommentsModel
    @Environment(\.sectionScrollProxy) private var scroll
    @State private var deleting: CommentThreadRow?
    @State private var reporting: ReportTarget?

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            if model.phase != .empty {
                MonacoSectionHeader(CommentsCopy.title, count: model.commentCount)
                    .secondaryCount()
                    .padding(.horizontal, MonacoTheme.Space.gutter)
            }
            content
        }
        .task(id: model.scrollRequest) { await scrollToRequest() }
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
            ForEach(model.entries) { entry in
                switch entry {
                case .row(let row):
                    commentRow(row)
                case .fold(let rootID, let count, let isExpanded):
                    CommentFoldButton(rootID: rootID, count: count, isExpanded: isExpanded) {
                        if isExpanded { model.collapse(rootID: rootID) } else { model.expand(rootID: rootID) }
                    }
                    .id(entry.id)
                }
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

extension CommentThreadView {
    fileprivate func commentRow(_ row: CommentThreadRow) -> some View {
        CommentRow(
            row: row,
            reactions: model.reactions(for: row),
            isHighlighted: model.highlightedID == row.id,
            canReply: !model.canCommentKnown || model.canComment,
            onReply: { model.presentReplySheet(.comment(row)) },
            onToggleReaction: { emoji in Task { await model.react(emoji, to: row) } },
            onDoubleTap: { Task { await model.doubleTap(row) } },
            onJump: { model.jump(to: $0) },
            onDelete: row.comment.isMine && !row.isDeleted ? { deleting = row } : nil,
            onReport: !row.comment.isMine && !row.isDeleted && !row.isSystem
                ? { reporting = ReportTarget(kind: .comment, targetID: row.id) } : nil
        )
        .id(row.id)
    }

    fileprivate func scrollToRequest() async {
        guard let request = model.scrollRequest else { return }
        await Task.yield()
        withAnimation { scroll?.scrollTo(request.id, anchor: .center) }
    }
}

private struct CommentFoldButton: View {
    let rootID: String
    let count: Int
    let isExpanded: Bool
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            HStack(spacing: MonacoTheme.Space.s) {
                Text(isExpanded ? CommentsCopy.hideReplies : CommentsCopy.viewReplies(count))
                    .font(MonacoTheme.Typo.headline)
                Image(monaco: .chevronDown)
                    .font(MonacoTheme.Typo.caption)
                    .rotationEffect(.degrees(isExpanded ? 180 : 0))
            }
            .foregroundStyle(MonacoTheme.brand)
            .frame(minHeight: CommentRowMetrics.replyTarget, alignment: .leading)
            .frame(maxWidth: .infinity, alignment: .leading)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .padding(.leading, CommentThreadLayout.textLeading(level: isExpanded ? 1 : 0))
        .padding(.trailing, MonacoTheme.Space.gutter)
        .accessibilityLabel(isExpanded ? CommentsCopy.hideReplies : CommentsCopy.viewReplies(count))
        .accessibilityIdentifier("comment-fold-\(rootID)")
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

extension View {
    func commentNotices(_ model: CommentsModel) -> some View {
        modifier(CommentNotices(model: model))
    }
}

private struct CommentNotices: ViewModifier {
    let model: CommentsModel
    @Environment(ToastCenter.self) private var toasts

    func body(content: Content) -> some View {
        content
            .onChange(of: model.noticeTick) { _, _ in
                switch model.notice {
                case .failure(let error): toasts.show(error)
                case .deleted: toasts.show(success: CommentsCopy.deleted)
                case nil: break
                }
            }
            .onChange(of: model.reactionNoticeTick) { _, _ in
                if let message = model.reactionNotice { toasts.current = MonacoToast(message: message) }
            }
    }
}
