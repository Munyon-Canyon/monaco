import MonacoCore
import SwiftUI
import Testing

@testable import Monaco

struct MarketsLayoutTests {
    @Test func aMoverTileIs244WideAtTheDefaultTextSize() {
        #expect(MoverTile.width(for: .large) == 244)
        #expect(MoverTile.width(for: .xxLarge) == 244)
    }

    @Test func aMoverTileGrowsTo285FromXXXLarge() {
        #expect(MoverTile.width(for: .xxxLarge) == 285)
    }

    @Test func atAccessibilityTextSizesTheRailStacksAndTilesTakeTheFullWidth() {
        for size in [DynamicTypeSize.accessibility1, .accessibility3, .accessibility5] {
            #expect(MoverTile.width(for: size) == nil)
        }
    }

    @Test func aMarketsRowIs64TallWithA40PointLogoAndA60PointSparkline() {
        #expect(MarketsRowMetrics.height == 64)
        #expect(MarketsRowMetrics.logo == 40)
        #expect(MarketsRowMetrics.sparklineWidth == 60)
    }

    @Test func theChipsAreAllWatchlistAndPreIpoOnly() {
        #expect(MonacoCore.StocksTabModel.availableBrowses.map(\.title) == ["All", "Watchlist", "Pre-IPO"])
        let count = MonacoCore.StocksTabModel.availableBrowses.count
        #expect(MonacoChipBar<MonacoCore.StocksTabModel.Browse>.fits(count, maxVisible: 4))
    }
}
