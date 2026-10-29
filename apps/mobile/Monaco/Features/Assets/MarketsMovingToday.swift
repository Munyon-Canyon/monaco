import MonacoCore
import SwiftUI

struct MarketsMovingToday: View {
    let movers: [MarketAsset]
    let open: (MarketAsset) -> Void
    let seeAll: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            MonacoSectionHeader(
                "Moving today", trailing: "See all", actionIdentifier: "markets-movers-see-all", action: seeAll
            )
            .padding(.horizontal, MonacoTheme.Space.gutter)
            ScrollView(.horizontal, showsIndicators: false) {
                LazyHStack(spacing: MonacoTheme.Space.sm) {
                    ForEach(movers) { asset in
                        MoverTile(asset: asset) { open(asset) }
                    }
                }
                .scrollTargetLayout()
            }
            .contentMargins(.horizontal, MonacoTheme.Space.gutter, for: .scrollContent)
            .scrollTargetBehavior(.viewAligned)
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("markets-movers")
    }
}
