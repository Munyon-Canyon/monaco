import SwiftUI

enum CabalsBoardSlot: CabalsTabSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
            MonacoSectionHeader("Top cabals")
            Text("Ranked by return across everyone on Monaco")
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
            Text("Rankings show up here soon.")
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
                .padding(.top, MonacoTheme.Space.xs)
                .accessibilityIdentifier("cabals-board-coming")
        }
        .padding(.horizontal, MonacoTheme.Space.m)
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("cabals-board")
    }
}
