import SwiftUI

enum RemindMetrics {
    static let capsuleHeight: CGFloat = 32
    static let target: CGFloat = 44
    static let glyph: CGFloat = 17
    static let gap: CGFloat = 6
    static let identifier = "remind-at-close"
}

struct RemindControl: View {
    let isOn: Bool
    var offTitle: String = "Remind me at close"
    let onTitle: String
    let action: () -> Void

    var body: some View {
        Button {
            Haptics.selection()
            action()
        } label: {
            HStack(spacing: RemindMetrics.gap) {
                Image(systemName: isOn ? "bell.fill" : "bell")
                    .font(.system(size: RemindMetrics.glyph))
                Text(isOn ? onTitle : offTitle)
                    .font(MonacoTheme.Typo.subheadStrong)
                    .lineLimit(2)
            }
            .foregroundStyle(isOn ? MonacoTheme.tint : MonacoTheme.primaryText)
            .padding(.horizontal, MonacoTheme.Space.sm)
            .frame(minHeight: RemindMetrics.capsuleHeight)
            .background(MonacoTheme.surfaceSunken, in: Capsule())
            .frame(minHeight: RemindMetrics.target)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityIdentifier(RemindMetrics.identifier)
        .accessibilityAddTraits(isOn ? .isSelected : [])
    }
}
