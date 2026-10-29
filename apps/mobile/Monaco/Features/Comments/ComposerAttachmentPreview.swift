import MonacoCore
import SwiftUI

struct ComposerAttachmentPreview: View {
    let draft: CommentAttachmentDraft
    let onRemove: () -> Void

    private var shape: RoundedRectangle {
        RoundedRectangle(cornerRadius: MonacoTheme.Radius.card, style: .continuous)
    }

    var body: some View {
        HStack(spacing: MonacoTheme.Space.s) {
            content
                .frame(maxWidth: .infinity, alignment: .leading)
            Button(action: onRemove) {
                Image(monaco: .close)
                    .font(MonacoTheme.Typo.subheadStrong)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .frame(width: CommentRowMetrics.replyTarget, height: CommentRowMetrics.replyTarget)
                    .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityLabel("Remove the \(draft.kindName)")
            .accessibilityIdentifier("composer-attachment-remove")
        }
        .padding(.leading, MonacoTheme.Space.sm)
        .background(shape.fill(MonacoTheme.surfaceSunken))
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("composer-attachment-preview")
    }

    @ViewBuilder private var content: some View {
        switch draft {
        case .thesis:
            HStack(spacing: MonacoTheme.Space.s) {
                Image(monaco: .thesis)
                    .font(MonacoTheme.Typo.subhead)
                    .foregroundStyle(MonacoTheme.secondaryText)
                Text("Quoting the thesis")
                    .font(MonacoTheme.Typo.subhead)
                    .foregroundStyle(MonacoTheme.primaryText)
            }
        }
    }
}

extension CommentsModel {
    func pick(_ draft: CommentAttachmentDraft, toasts: ToastCenter) {
        if let replaced = pick(draft) { toasts.show(success: replaced) }
    }
}

struct ComposerAttachmentSlot: View {
    let model: CommentsModel

    var body: some View {
        if let draft = model.attachmentDraft {
            ComposerAttachmentPreview(draft: draft, onRemove: { model.removeAttachment() })
        }
    }
}
