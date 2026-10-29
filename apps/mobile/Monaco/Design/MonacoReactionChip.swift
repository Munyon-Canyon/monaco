import MonacoCore
import SwiftUI

struct MonacoReactionChip: View {
    enum Kind: Equatable {
        case emoji(String, count: Int, mine: Bool)
        case add
        case more(Int)
        case reply(Int)
    }

    enum Size: Equatable {
        case regular
        case compact
    }

    let kind: Kind
    let size: Size
    let action: () -> Void

    @ScaledMetric(relativeTo: .subheadline) private var glyphSize: CGFloat = 13

    init(emoji: String, count: Int, mine: Bool, size: Size = .regular, action: @escaping () -> Void) {
        kind = .emoji(emoji, count: count, mine: mine)
        self.size = size
        self.action = action
    }

    private init(kind: Kind, size: Size = .regular, action: @escaping () -> Void) {
        self.kind = kind
        self.size = size
        self.action = action
    }

    static func add(action: @escaping () -> Void) -> MonacoReactionChip {
        MonacoReactionChip(kind: .add, action: action)
    }

    static func more(_ count: Int, size: Size = .regular, action: @escaping () -> Void) -> MonacoReactionChip {
        MonacoReactionChip(kind: .more(count), size: size, action: action)
    }

    static func reply(count: Int, action: @escaping () -> Void) -> MonacoReactionChip {
        MonacoReactionChip(kind: .reply(count), action: action)
    }

    var isMine: Bool {
        if case .emoji(_, _, let mine) = kind { return mine }
        return false
    }

    var spokenLabel: String {
        switch kind {
        case .emoji(let emoji, let count, let mine):
            let noun = count == 1 ? "reaction" : "reactions"
            return "\(emoji.reactionName), \(count) \(noun)" + (mine ? ", yours" : "")
        case .add: return "Add a reaction"
        case .more(let count): return "\(count) more \(count == 1 ? "reaction" : "reactions")"
        case .reply(let count):
            return count == 0 ? "Reply" : count == 1 ? "1 reply" : "\(count) replies"
        }
    }

    private typealias Metrics = MonacoTheme.ReactionChipMetrics

    private var visibleHeight: CGFloat {
        size == .compact ? Metrics.compactVisibleHeight : Metrics.visibleHeight
    }

    private var labelFont: Font {
        size == .compact ? MonacoTheme.Typo.subhead : MonacoTheme.Typo.subheadStrong
    }

    var body: some View {
        Button(action: action) {
            label
                .font(labelFont)
                .foregroundStyle(isMine ? MonacoTheme.brand : MonacoTheme.primaryText)
                .lineLimit(1)
                .padding(.horizontal, Metrics.horizontalPadding)
                .frame(minWidth: Metrics.minWidth, minHeight: visibleHeight)
                .background(Capsule().fill(MonacoTheme.surfaceSunken))
                .overlay { if isMine { Capsule().strokeBorder(MonacoTheme.brand, lineWidth: 1.5) } }
                .padding(.vertical, (Metrics.tapSize - visibleHeight) / 2)
                .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(spokenLabel)
        .accessibilityAddTraits(isMine ? [.isButton, .isSelected] : .isButton)
        .modifier(RemoveReactionAction(isMine: isMine, action: action))
    }

    @ViewBuilder private var label: some View {
        switch kind {
        case .emoji(let emoji, let count, _):
            HStack(spacing: Metrics.gap) {
                Text(emoji)
                Text("\(count)").monospacedDigit()
            }
        case .add:
            Image(monaco: .plus).font(.system(size: glyphSize, weight: .semibold))
        case .more(let count):
            Text("+\(count)").monospacedDigit()
        case .reply(let count):
            HStack(spacing: Metrics.gap) {
                Image(monaco: .chat).font(.system(size: glyphSize, weight: .semibold))
                if count > 0 { Text("\(count)").monospacedDigit() }
            }
        }
    }
}

private struct RemoveReactionAction: ViewModifier {
    let isMine: Bool
    let action: () -> Void

    func body(content: Content) -> some View {
        if isMine {
            content.accessibilityAction(named: "Remove your reaction", action)
        } else {
            content
        }
    }
}

extension String {
    var reactionName: String {
        EmojiCatalog.bundled.name(for: self) ?? unicodeScalars.first?.properties.name?.lowercased() ?? self
    }

    var reactionHex: String {
        unicodeScalars.map { String($0.value, radix: 16, uppercase: true) }.joined(separator: "-")
    }
}
