import MonacoAPI
import MonacoCore
import SwiftUI

struct ReactionRow<Trailing: View>: View {
    static var maxChips: Int { 3 }

    let target: ReactionTarget
    let reactions: [Components.Schemas.Reaction]
    let showsAdd: Bool
    let size: MonacoReactionChip.Size
    let onToggle: (String) -> Void
    let onOpenTray: () -> Void
    let trailing: () -> Trailing

    @State private var isTrayOpen: Bool
    @State private var isPickerOpen = false

    init(
        target: ReactionTarget,
        reactions: [Components.Schemas.Reaction],
        showsAdd: Bool = true,
        size: MonacoReactionChip.Size = .regular,
        startsOpen: Bool = false,
        onToggle: @escaping (String) -> Void, onOpenTray: @escaping () -> Void = {},
        @ViewBuilder trailing: @escaping () -> Trailing
    ) {
        self.target = target
        self.reactions = reactions
        self.showsAdd = showsAdd
        self.size = size
        self.onToggle = onToggle
        self.onOpenTray = onOpenTray
        self.trailing = trailing
        _isTrayOpen = State(initialValue: startsOpen)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            ViewThatFits(in: .horizontal) {
                ForEach(Self.limits(for: reactions.count), id: \.self) { limit in
                    line(limit: limit)
                }
            }
            if isTrayOpen {
                QuickReactionTray(target: target, onOpenPicker: { isPickerOpen = true }, onPick: onToggle) {
                    isTrayOpen = false
                }
                .transition(.opacity)
            }
        }
        .animation(.easeOut(duration: 0.15), value: isTrayOpen)
        .sheet(isPresented: $isPickerOpen) {
            EmojiPickerSheet(onPick: onToggle)
        }
    }

    private func line(limit: Int) -> some View {
        HStack(spacing: MonacoTheme.ReactionChipMetrics.gap) {
            ForEach(Self.chips(for: reactions, limit: limit)) { chip in
                switch chip {
                case .emoji(let reaction):
                    MonacoReactionChip(
                        emoji: reaction.emoji, count: Int(reaction.count), mine: reaction.mine, size: size
                    ) {
                        Haptics.selection()
                        onToggle(reaction.emoji)
                    }
                    .accessibilityIdentifier("reaction-chip-\(target.key)-\(reaction.emoji.reactionHex)")
                case .more(let count):
                    MonacoReactionChip.more(count, size: size, action: openTray)
                        .accessibilityIdentifier("reaction-more-\(target.key)")
                }
            }
            if showsAdd && !isTrayOpen {
                MonacoReactionChip.add(action: openTray)
                    .accessibilityIdentifier("reaction-add-\(target.key)")
            }
            trailing()
        }
        .fixedSize(horizontal: true, vertical: false)
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private func openTray() {
        Haptics.selection()
        isTrayOpen = true
        onOpenTray()
    }

    enum Chip: Identifiable, Equatable {
        case emoji(Components.Schemas.Reaction)
        case more(Int)

        var id: String {
            switch self {
            case .emoji(let reaction): reaction.emoji
            case .more: "more"
            }
        }
    }

    static func chips(for reactions: [Components.Schemas.Reaction], limit: Int = maxChips) -> [Chip] {
        let ranked = reactions.enumerated().sorted { ($0.element.count, $1.offset) > ($1.element.count, $0.offset) }
            .map(\.element)
        guard ranked.count > limit else { return ranked.map(Chip.emoji) }
        let rest = ranked.dropFirst(limit - 1).reduce(0) { $0 + Int($1.count) }
        return ranked.prefix(limit - 1).map(Chip.emoji) + [.more(rest)]
    }

    static func limits(for count: Int) -> [Int] {
        Array((1...min(max(count, 1), maxChips)).reversed())
    }
}
