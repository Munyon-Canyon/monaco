import MonacoCore
import SwiftUI

struct CommentComposerBar: View {
    let model: CommentsModel
    var onDidStandDown: () -> Void = {}

    var body: some View {
        if model.canComment {
            CommentComposer(
                replyTarget: model.replyTarget,
                isPosting: model.isPosting,
                onCancelReply: { model.cancelReply() },
                onPost: { await model.post($0) },
                onDidStandDown: onDidStandDown
            )
        } else {
            Text(CommentsCopy.membersOnly)
                .font(MonacoTheme.Typo.callout)
                .foregroundStyle(MonacoTheme.muted)
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .padding(.vertical, MonacoTheme.Space.m)
                .background(MonacoTheme.canvas.ignoresSafeArea(edges: .bottom))
                .overlay(alignment: .top) { MonacoRule() }
                .accessibilityIdentifier("comment-members-only")
        }
    }
}

struct CommentComposer: View {
    let replyTarget: CommentThreadRow?
    let isPosting: Bool
    let onCancelReply: () -> Void
    let onPost: (String) async -> Bool
    var onDidStandDown: () -> Void = {}

    @State private var text = ""
    @FocusState private var focused: Bool

    private var draft: CommentDraft {
        CommentDraft(text: text)
    }

    private var canPost: Bool {
        draft.body != nil && !isPosting
    }

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
            if let replyTarget {
                HStack {
                    Text(CommentsCopy.replyingTo(replyTarget.authorName))
                        .font(MonacoTheme.Typo.captionStrong)
                        .foregroundStyle(MonacoTheme.muted)
                    Spacer()
                    Button {
                        onCancelReply()
                    } label: {
                        Image(systemName: "xmark")
                            .font(.system(size: 12, weight: .semibold))
                            .foregroundStyle(MonacoTheme.muted)
                            .frame(width: 44, height: 44)
                            .contentShape(Rectangle())
                    }
                    .buttonStyle(.plain)
                    .padding(.trailing, -MonacoTheme.Space.sm)
                    .accessibilityLabel("Cancel reply")
                    .accessibilityIdentifier("comment-composer-cancel-reply")
                }
            }

            HStack(alignment: .bottom, spacing: MonacoTheme.Space.s) {
                TextField(
                    "",
                    text: $text,
                    prompt: Text(CommentsCopy.placeholder).foregroundStyle(MonacoTheme.disabledLabel),
                    axis: .vertical
                )
                .font(MonacoTheme.Typo.body)
                .foregroundStyle(MonacoTheme.ink)
                .tint(MonacoTheme.ink)
                .lineLimit(1...5)
                .focused($focused)
                .padding(.horizontal, MonacoTheme.Space.m)
                .padding(.vertical, MonacoTheme.Space.sm)
                .frame(minHeight: 44)
                .background(
                    MonacoTheme.surfaceSunken,
                    in: RoundedRectangle(cornerRadius: MonacoTheme.Radius.field, style: .continuous)
                )
                .accessibilityIdentifier("comment-composer-field")

                Button {
                    guard let body = draft.body else { return }
                    Task {
                        guard await onPost(body) else { return }
                        text = ""
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
    }
}

struct ComposerSendDisc: View {
    let isLive: Bool
    let isSending: Bool

    var body: some View {
        ZStack {
            Circle()
                .fill(isLive ? MonacoTheme.brandFill : MonacoTheme.surfaceSunken)
            if isSending {
                ProgressView()
                    .tint(MonacoTheme.onBrand)
            } else {
                Image(systemName: "arrow.up")
                    .font(.system(size: 17, weight: .semibold))
                    .foregroundStyle(isLive ? MonacoTheme.onBrand : MonacoTheme.disabledLabel)
            }
        }
        .frame(width: 44, height: 44)
        .animation(.easeOut(duration: 0.15), value: isLive)
    }
}
