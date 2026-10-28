import MonacoCore
import SwiftUI

struct QuickReactionTray: View {
    static let autoCloseAfter: Duration = .seconds(6)
    static let emojiSize: CGFloat = 40

    let target: ReactionTarget
    var onOpenPicker: (() -> Void)?
    let onPick: (String) -> Void
    let onClose: () -> Void

    @State private var restingY: CGFloat?

    private typealias Metrics = MonacoTheme.ReactionChipMetrics

    var body: some View {
        HStack(spacing: 0) {
            ForEach(QuickReactions.six, id: \.self) { emoji in
                Button {
                    Haptics.selection()
                    onClose()
                    onPick(emoji)
                } label: {
                    Text(emoji)
                        .font(.title2)
                        .frame(width: Self.emojiSize, height: Self.emojiSize)
                        .frame(width: Metrics.tapSize, height: Metrics.tapSize)
                        .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .accessibilityLabel(emoji.reactionName)
                .accessibilityIdentifier("reaction-tray-\(emoji.reactionHex)")
            }
            if let onOpenPicker {
                Button {
                    Haptics.selection()
                    onClose()
                    onOpenPicker()
                } label: {
                    Image(monaco: .plus)
                        .font(MonacoTheme.Typo.subheadStrong)
                        .foregroundStyle(MonacoTheme.primaryText)
                        .frame(width: Self.emojiSize, height: Self.emojiSize)
                        .background(Circle().fill(MonacoTheme.border))
                        .frame(width: Metrics.tapSize, height: Metrics.tapSize)
                        .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .accessibilityLabel("More reactions")
                .accessibilityIdentifier("reaction-tray-more-\(target.key)")
            }
        }
        .background(Capsule().fill(MonacoTheme.surfaceSunken))
        .background {
            Color.clear.frame(width: 4_000, height: 4_000).contentShape(Rectangle()).onTapGesture(perform: onClose)
        }
        .onGeometryChange(
            for: CGFloat.self,
            of: { $0.frame(in: .global).minY },
            action: { y in
                guard let restingY else {
                    self.restingY = y
                    return
                }
                if abs(y - restingY) > 1 { onClose() }
            }
        )
        .task {
            try? await Task.sleep(for: Self.autoCloseAfter)
            if !Task.isCancelled { onClose() }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("reaction-tray-\(target.key)")
    }
}
