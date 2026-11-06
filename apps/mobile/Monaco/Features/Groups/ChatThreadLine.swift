import MonacoAPI
import MonacoCore
import SwiftUI

enum ChatThreadLineMetrics {
    static let faceSize: CGFloat = 20
    static let faceOverlap: CGFloat = 6
    static let faceRing: CGFloat = 1.5
}

struct ChatThreadLine: View {
    let message: ChatMessage
    let now: Date
    let open: (() -> Void)?

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    static func faces(of message: ChatMessage) -> [Components.Schemas.ChatAuthor] {
        Array(message.lastRepliers.prefix(ChatThreadCopy.maxFaces))
    }

    static func ageText(of message: ChatMessage, now: Date) -> String? {
        ChatThreadCopy.lineAge(of: message.lastReplyAt, now: now)
    }

    static func spokenLabel(of message: ChatMessage, now: Date) -> String {
        ChatThreadCopy.lineSpoken(
            replyCount: message.replyCount, lastReplyAt: message.lastReplyAt,
            names: faces(of: message).map(\.displayName), now: now)
    }

    var body: some View {
        if let open {
            Button(action: open) { line }
                .buttonStyle(.plain)
                .accessibilityLabel(Self.spokenLabel(of: message, now: now))
                .accessibilityIdentifier("chat-replies-\(message.id)")
        } else {
            line
                .accessibilityElement(children: .ignore)
                .accessibilityLabel(Self.spokenLabel(of: message, now: now))
                .accessibilityIdentifier("chat-replies-\(message.id)")
        }
    }

    private var line: some View {
        content
            .frame(minHeight: MonacoButtonMetrics.hitTarget)
            .contentShape(Rectangle())
    }

    @ViewBuilder private var content: some View {
        if dynamicTypeSize.isAccessibilitySize {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                faceStack
                labels
            }
        } else {
            HStack(spacing: MonacoTheme.Space.s) {
                faceStack
                labels
            }
        }
    }

    @ViewBuilder private var faceStack: some View {
        let faces = Self.faces(of: message)
        if !faces.isEmpty {
            HStack(spacing: -ChatThreadLineMetrics.faceOverlap) {
                ForEach(faces, id: \.id) { author in
                    MonacoAvatar(
                        photoURL: author.photoUrl, displayName: author.displayName,
                        size: ChatThreadLineMetrics.faceSize, seed: author.id
                    )
                    .overlay {
                        Circle().strokeBorder(MonacoTheme.canvas, lineWidth: ChatThreadLineMetrics.faceRing)
                    }
                }
            }
        }
    }

    private var labels: some View {
        HStack(spacing: MonacoTheme.Space.s) {
            Text(GroupChatCopy.replies(message.replyCount))
                .font(Font.footnote.weight(.semibold))
                .foregroundStyle(MonacoTheme.primaryText)
            if let age = Self.ageText(of: message, now: now) {
                Text(age)
                    .font(MonacoTheme.Typo.subhead)
                    .foregroundStyle(MonacoTheme.secondaryText)
            }
            if open != nil {
                Image(systemName: "chevron.right")
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.tertiaryText)
                    .accessibilityHidden(true)
            }
        }
    }
}
