import MonacoAPI
import SwiftUI

enum ChatTapbackMetrics {
    static let overlap: CGFloat = 6
    static let out: CGFloat = 6
    static let ring: CGFloat = 2
    static let topSpace: CGFloat = 10
    static let hitInset: CGFloat = 14
    static let maxEmoji = 3
    static let minMenuWidth: CGFloat = 220
}

struct ChatTapbackBadge: View {
    let messageID: String
    let reactions: [Components.Schemas.Reaction]
    let onTap: () -> Void
    let onLongPress: () -> Void

    var body: some View {
        HStack(spacing: MonacoTheme.Space.xs) {
            Text(Self.summaryEmoji(reactions))
                .font(MonacoTheme.Typo.subhead)
            Text("\(Self.total(reactions))")
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.secondaryText)
        }
        .padding(.horizontal, MonacoTheme.Space.s)
        .padding(.vertical, MonacoTheme.Space.xs)
        .background(Capsule().fill(MonacoTheme.surfaceSunken))
        .padding(ChatTapbackMetrics.ring)
        .background(Capsule().fill(MonacoTheme.canvas))
        .padding(ChatTapbackMetrics.hitInset)
        .contentShape(Rectangle())
        .padding(-ChatTapbackMetrics.hitInset)
        .onTapGesture(perform: onTap)
        .onLongPressGesture(perform: onLongPress)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(Self.accessibilityLabel(reactions))
        .accessibilityAddTraits(.isButton)
        .accessibilityIdentifier("chat-tapback-\(messageID)")
    }

    static func ranked(_ reactions: [Components.Schemas.Reaction]) -> [Components.Schemas.Reaction] {
        let live = reactions.filter { $0.count > 0 }
        return live.enumerated().sorted {
            $0.element.count != $1.element.count ? $0.element.count > $1.element.count : $0.offset < $1.offset
        }.map(\.element)
    }

    static func summaryEmoji(_ reactions: [Components.Schemas.Reaction]) -> String {
        ranked(reactions).prefix(ChatTapbackMetrics.maxEmoji).map(\.emoji).joined()
    }

    static func total(_ reactions: [Components.Schemas.Reaction]) -> Int {
        reactions.reduce(0) { $0 + max(Int($1.count), 0) }
    }

    static func accessibilityLabel(_ reactions: [Components.Schemas.Reaction]) -> String {
        let count = total(reactions)
        let noun = count == 1 ? "reaction" : "reactions"
        let parts = ranked(reactions).map { "\($0.emoji) ×\($0.count)" }.joined(separator: ", ")
        return "\(count) \(noun): \(parts)"
    }

    static func tapAction(_ reactions: [Components.Schemas.Reaction]) -> TapAction {
        if let mine = ranked(reactions).first(where: \.mine) { return .remove(mine.emoji) }
        return .openTray
    }

    enum TapAction: Equatable {
        case remove(String)
        case openTray
    }
}
