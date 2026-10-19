import MonacoCore
import SwiftUI

struct CommentComposerBar: View {
    let model: CommentsModel
    var onDidStandDown: () -> Void = {}

    var body: some View {
        if model.canCommentFailed {
            MonacoErrorRow(thing: "the comment box", identifier: "comment-composer-error") {
                Task { await model.retryCanComment() }
            }
            .background(MonacoTheme.canvas.ignoresSafeArea(edges: .bottom))
            .overlay(alignment: .top) { MonacoRule() }
        } else if !model.canCommentKnown {
            EmptyView()
        } else if model.canComment {
            CommentComposer(
                replyTarget: model.replyTarget,
                isPosting: model.isPosting,
                lastPostedID: model.lastPostedID,
                onCancelReply: { model.cancelReply() },
                onPost: { await model.post($0) },
                onDidStandDown: onDidStandDown
            )
        } else {
            MonacoNotice(.info, message: CommentsCopy.membersOnly)
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .padding(.vertical, MonacoTheme.Space.s)
                .frame(maxWidth: .infinity)
                .background(MonacoTheme.canvas.ignoresSafeArea(edges: .bottom))
                .overlay(alignment: .top) { MonacoRule() }
                .accessibilityIdentifier("comment-members-only")
        }
    }
}

struct CommentComposer: View {
    let replyTarget: CommentThreadRow?
    let isPosting: Bool
    let lastPostedID: String?
    let onCancelReply: () -> Void
    let onPost: (String) async -> Bool
    var onDidStandDown: () -> Void = {}

    @State private var text = ""
    @State private var postFailed = false
    @State private var lastPostedBeforeSend: String?
    @FocusState private var focused: Bool

    private var draft: CommentDraft {
        CommentDraft(text: text)
    }

    private var canPost: Bool {
        draft.body != nil && !isPosting
    }

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            notice
            if let replyTarget { replyRow(replyTarget) }
            HStack(alignment: .bottom, spacing: MonacoTheme.Space.s) {
                field
                sendButton
            }
            if case .tooLong = draft {
                Text(CommentsCopy.tooLong)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.loss)
                    .accessibilityIdentifier("comment-composer-too-long")
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.vertical, MonacoTheme.Space.s)
        .background(MonacoTheme.canvas.ignoresSafeArea(edges: .bottom))
        .overlay(alignment: .top) {
            MonacoRule()
        }
        .onChange(of: replyTarget?.id) { _, newValue in
            if newValue != nil { focused = true }
        }
        .onChange(of: isPosting) { wasPosting, nowPosting in
            if nowPosting {
                lastPostedBeforeSend = lastPostedID
                postFailed = false
            } else if wasPosting {
                postFailed = lastPostedID == lastPostedBeforeSend
            }
        }
        .onChange(of: text) { _, _ in postFailed = false }
    }

    @ViewBuilder private var notice: some View {
        if isPosting {
            MonacoNotice(.info, message: CommentsCopy.sending)
                .accessibilityIdentifier("comment-composer-sending")
        } else if postFailed {
            MonacoNotice(.error, message: CommentsCopy.sendFailed)
                .accessibilityIdentifier("comment-composer-failed")
        }
    }

    private func replyRow(_ target: CommentThreadRow) -> some View {
        HStack {
            Text(CommentsCopy.replyingTo(target.authorName))
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
            Spacer()
            Button {
                onCancelReply()
            } label: {
                Image(monaco: .close)
                    .font(MonacoTheme.Typo.captionStrong)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .frame(width: CommentRowMetrics.replyTarget, height: CommentRowMetrics.replyTarget)
                    .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .padding(.trailing, -MonacoTheme.Space.sm)
            .accessibilityLabel("Cancel reply")
            .accessibilityIdentifier("comment-composer-cancel-reply")
        }
    }

    private var field: some View {
        let shape = RoundedRectangle(cornerRadius: MonacoTheme.Radius.cardTight, style: .continuous)
        return TextField(
            "",
            text: $text,
            prompt: Text(CommentsCopy.placeholder).foregroundStyle(MonacoTheme.secondaryText),
            axis: .vertical
        )
        .font(MonacoTheme.Typo.body)
        .foregroundStyle(MonacoTheme.primaryText)
        .tint(MonacoTheme.primaryText)
        .lineLimit(1...5)
        .focused($focused)
        .padding(.horizontal, MonacoTheme.Space.m)
        .padding(.vertical, MonacoTheme.Space.sm)
        .frame(minHeight: CommentRowMetrics.replyTarget)
        .background(shape.fill(MonacoTheme.surfaceSunken))
        .overlay(shape.strokeBorder(MonacoTheme.border, lineWidth: 1))
        .accessibilityIdentifier("comment-composer-field")
    }

    private var sendButton: some View {
        Button {
            guard let body = draft.body else { return }
            Task {
                guard await onPost(body) else { return }
                text = CommentDraft.field(text, afterPosting: body)
                guard text.isEmpty else { return }
                focused = false
                onDidStandDown()
            }
        } label: {
            ComposerSendDisc(isLive: canPost || isPosting, isSending: isPosting)
        }
        .buttonStyle(.plain)
        .disabled(!canPost)
        .accessibilityLabel(CommentsCopy.postAccessibility)
        .accessibilityIdentifier("comment-composer-send")
    }
}

struct ComposerSendDisc: View {
    let isLive: Bool
    let isSending: Bool

    var body: some View {
        ZStack {
            Circle()
                .fill(MonacoTheme.brandFill)
            if isSending {
                ProgressView()
                    .tint(MonacoTheme.onBrand)
            } else {
                Image(monaco: .up)
                    .font(MonacoTheme.Typo.subheadStrong)
                    .foregroundStyle(MonacoTheme.onBrand)
            }
        }
        .frame(width: CommentRowMetrics.replyTarget, height: CommentRowMetrics.replyTarget)
        .opacity(isLive ? 1 : ComposerSendDisc.idleOpacity)
        .animation(.easeOut(duration: 0.15), value: isLive)
    }

    static let idleOpacity = 0.45
}
