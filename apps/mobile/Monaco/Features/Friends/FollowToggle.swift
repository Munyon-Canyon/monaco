import SwiftUI

struct MonacoCompactButtonStyle: ButtonStyle {
    var isProminent = false

    @Environment(\.isEnabled) private var isEnabled

    static let height: CGFloat = 36

    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(MonacoTheme.Typo.callout.weight(.semibold))
            .lineLimit(1)
            .minimumScaleFactor(0.8)
            .padding(.horizontal, MonacoTheme.Space.sm)
            .frame(minHeight: Self.height)
            .foregroundStyle(
                isProminent
                    ? MonacoTheme.primaryButtonLabel
                    : (isEnabled ? MonacoTheme.secondaryButtonLabel : MonacoTheme.disabledLabel)
            )
            .background(
                Capsule().fill(
                    isProminent
                        ? MonacoTheme.primaryButtonFill.opacity(
                            isEnabled ? 1 : MonacoPrimaryButtonStyle.disabledFillOpacity)
                        : MonacoTheme.secondaryButtonFill)
            )
            .overlay {
                if !isProminent { Capsule().strokeBorder(MonacoTheme.hairline, lineWidth: 1) }
            }
            .contentShape(Capsule())
            .opacity(configuration.isPressed ? 0.8 : 1)
    }
}

extension ButtonStyle where Self == MonacoCompactButtonStyle {
    static var monacoCompact: MonacoCompactButtonStyle { MonacoCompactButtonStyle() }
    static var monacoCompactProminent: MonacoCompactButtonStyle { MonacoCompactButtonStyle(isProminent: true) }
}

struct FollowToggle: View {
    let isFollowing: Bool
    var isBusy = false
    var action: (() -> Void)?
    let identifier: String

    var body: some View {
        if isFollowing {
            Button("Following") { action?() }
                .buttonStyle(.monacoCompact)
                .disabled(isBusy || action == nil)
                .accessibilityIdentifier(identifier)
        } else {
            Button("Follow") { action?() }
                .buttonStyle(.monacoCompactProminent)
                .disabled(isBusy || action == nil)
                .accessibilityIdentifier(identifier)
        }
    }
}
