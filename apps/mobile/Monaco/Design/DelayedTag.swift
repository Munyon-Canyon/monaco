import SwiftUI

struct DelayedTag: View {
    var body: some View {
        Text("Delayed")
            .font(MonacoTheme.Typo.caption)
            .foregroundStyle(MonacoTheme.muted)
            .padding(.horizontal, MonacoTheme.Space.s)
            .padding(.vertical, MonacoTheme.Space.xs)
            .background(MonacoTheme.surfaceSunken, in: Capsule())
            .fixedSize()
            .accessibilityIdentifier("stock-delayed-tag")
    }
}
