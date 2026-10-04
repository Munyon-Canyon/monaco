import SwiftUI

enum CabalHoldingsSlot: CabalSection {
    static let isLive = true

    static func body(for context: CabalContext) -> some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            MonacoSectionHeader("Holdings")
            Text("Holdings show up here soon.")
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
                .accessibilityIdentifier("cabal-holdings-coming")
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}
