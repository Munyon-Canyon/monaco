import SwiftUI

enum CabalValueChartSlot: CabalSection {
    static let isLive = true

    static func body(for context: CabalContext) -> some View {
        CabalInkBand {
            Rectangle()
                .fill(MonacoTheme.onHeroHairline)
                .frame(height: 1)
                .padding(.vertical, MonacoTheme.Space.m)
                .accessibilityHidden(true)
            CabalInkCaption("The pot's history shows up here soon.", id: "cabal-value-chart-coming")
        }
    }
}
