import MonacoCore
import SwiftUI

struct MonacoReactionChip: View {
    enum Kind: Equatable {
        case emoji(String, count: Int, mine: Bool)
        case add
        case more(Int)
        case reply(Int)
    }

    let kind: Kind
    let action: () -> Void

    @ScaledMetric(relativeTo: .subheadline) private var glyphSize: CGFloat = 13

    init(emoji: String, count: Int, mine: Bool, action: @escaping () -> Void) {
        kind = .emoji(emoji, count: count, mine: mine)
        self.action = action
    }

    private init(kind: Kind, action: @escaping () -> Void) {
        self.kind = kind
        self.action = action
    }

    static func add(action: @escaping () -> Void) -> MonacoReactionChip {
        MonacoReactionChip(kind: .add, action: action)
    }

    static func more(_ count: Int, action: @escaping () -> Void) -> MonacoReactionChip {
        MonacoReactionChip(kind: .more(count), action: action)
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

    var body: some View {
        Button(action: action) {
            label
                .font(MonacoTheme.Typo.subheadStrong)
                .foregroundStyle(isMine ? MonacoTheme.brand : MonacoTheme.primaryText)
                .lineLimit(1)
                .padding(.horizontal, Metrics.horizontalPadding)
                .frame(minWidth: Metrics.minWidth, minHeight: Metrics.visibleHeight)
                .background(Capsule().fill(MonacoTheme.surfaceSunken))
                .overlay { if isMine { Capsule().strokeBorder(MonacoTheme.brand, lineWidth: 1.5) } }
                .padding(.vertical, (Metrics.tapSize - Metrics.visibleHeight) / 2)
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
        unicodeScalars.first?.properties.name?.lowercased() ?? self
    }

    var reactionHex: String {
        unicodeScalars.map { String($0.value, radix: 16, uppercase: true) }.joined(separator: "-")
    }
}
