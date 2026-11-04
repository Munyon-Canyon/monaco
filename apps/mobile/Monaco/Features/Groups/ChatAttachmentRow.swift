import MonacoAPI
import MonacoCore
import SwiftUI

struct ChatAttachmentRow: View {
    enum Kind: Equatable {
        case gif
        case stock
        case thesis
        case poll
    }

    enum Content: Equatable {
        case text
        case deleted
        case attachment(Kind)
        case pending(Kind)
    }

    let row: ChatRow

    static func content(of row: ChatRow) -> Content {
        if row.message.deleted { return .deleted }
        if let attachment = row.attachment { return .attachment(kind(of: attachment)) }
        if let draft = row.pendingAttachment { return .pending(kind(of: draft)) }
        return .text
    }

    static func kind(of attachment: Components.Schemas.CommentAttachment) -> Kind {
        switch attachment {
        case .gif: .gif
        case .stock: .stock
        case .thesisQuote: .thesis
        case .poll: .poll
        }
    }

    static func kind(of draft: ChatAttachmentDraft) -> Kind {
        switch draft {
        case .gif: .gif
        case .stock: .stock
        case .thesisQuote: .thesis
        case .poll: .poll
        }
    }

    static func showsAttachment(_ row: ChatRow) -> Bool {
        switch content(of: row) {
        case .attachment, .pending: true
        case .text, .deleted: false
        }
    }

    static func showsTextAbove(_ row: ChatRow) -> Bool {
        showsAttachment(row) && !row.displayText.isEmpty
    }

    static func gifLabel(author: String, isMine: Bool, title: String) -> String {
        let who = isMine ? "You" : author
        let base = "GIF from \(who)"
        return title.isEmpty ? base : "\(base), \(title)"
    }

    var body: some View {
        content
            .accessibilityIdentifier("chat-attachment-\(row.id)")
    }

    @ViewBuilder private var content: some View {
        if let attachment = row.attachment {
            sent(attachment)
        } else if let draft = row.pendingAttachment {
            skeleton(draft)
        }
    }

    @ViewBuilder private func sent(_ attachment: Components.Schemas.CommentAttachment) -> some View {
        if case .gif(let gif) = attachment {
            AttachmentBubble(commentID: row.id, attachment: attachment)
                .accessibilityElement(children: .combine)
                .accessibilityLabel(
                    Self.gifLabel(author: row.message.author.displayName, isMine: row.isMine, title: gif.title))
        } else {
            AttachmentBubble(commentID: row.id, attachment: attachment)
        }
    }

    private func skeleton(_ draft: ChatAttachmentDraft) -> some View {
        let size = Self.skeletonSize(draft)
        let radius = Self.kind(of: draft) == .gif ? GifBubbleMetrics.radius : MonacoTheme.Radius.compact
        return RoundedRectangle(cornerRadius: radius, style: .continuous)
            .fill(MonacoTheme.surfaceSunken)
            .frame(width: size.width, height: size.height)
            .accessibilityHidden(true)
    }

    static func skeletonSize(_ draft: ChatAttachmentDraft) -> CGSize {
        let width = GifBubbleMetrics.width
        guard case .gif(let gif) = draft else { return CGSize(width: width, height: width * 3 / 4) }
        return GifBubbleMetrics.size(
            mediaWidth: CGFloat(gif.preview.width), mediaHeight: CGFloat(gif.preview.height), available: width)
    }
}
