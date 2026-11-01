import SwiftUI

struct SettingsToggleRow: View {
    let title: String
    let subtitle: String
    let identifier: String
    var isLast = false
    var isEnabled = true
    @Binding var isOn: Bool

    var body: some View {
        HStack(spacing: MonacoTheme.Space.sm) {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                Text(title)
                    .font(MonacoTheme.Typo.headline)
                    .foregroundStyle(MonacoTheme.primaryText)
                Text(subtitle)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.secondaryText)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            Toggle(title, isOn: $isOn)
                .labelsHidden()
                .tint(MonacoTheme.brandFill)
                .disabled(!isEnabled)
                .accessibilityIdentifier("\(identifier)-switch")
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.vertical, MonacoTheme.Space.sm)
        .frame(minHeight: MonacoRowLayout.minHeight)
        .overlay(alignment: .bottom) {
            if !isLast {
                MonacoRule().padding(.horizontal, MonacoTheme.Space.gutter)
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier(identifier)
    }
}

struct AnalyticsToggleRow: View {
    @State private var isSharing = AnalyticsConsent.isSharing

    var body: some View {
        SettingsToggleRow(
            title: AnalyticsConsent.title, subtitle: AnalyticsConsent.subtitle, identifier: "settings-analytics",
            isLast: true,
            isOn: Binding(
                get: { isSharing },
                set: { sharing in
                    isSharing = sharing
                    AnalyticsConsent.set(sharing: sharing)
                }))
    }
}
