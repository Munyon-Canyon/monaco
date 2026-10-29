import MonacoCore
import SwiftUI

struct AssetMarketStatsInfo: View {
    let facts: MarketStatsFacts

    var body: some View {
        MarketStatsSheet(facts: facts)
            .accessibilityIdentifier("asset-stats-info")
    }
}
