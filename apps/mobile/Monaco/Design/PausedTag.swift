import SwiftUI

struct PausedTag: View {
    var body: some View {
        Text("Paused")
            .font(MonacoTheme.Typo.caption)
            .foregroundStyle(MonacoTheme.muted)
            .padding(.horizontal, MonacoTheme.Space.s)
            .padding(.vertical, MonacoTheme.Space.xs)
            .background(MonacoTheme.surfaceSunken, in: Capsule())
            .fixedSize()
            .accessibilityIdentifier("stock-paused-tag")
    }
}
