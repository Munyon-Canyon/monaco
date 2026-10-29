import MonacoCore
import SwiftUI
import Testing

@testable import Monaco

struct MarketsLayoutTests {
    @Test func aMoverTileIs104By152() {
        #expect(MoverTile.width == 104)
        #expect(MoverTile.height == 152)
    }

    @Test func aMarketsRowIs64TallWithA40PointLogoAndA60PointSparkline() {
        #expect(MarketsRowMetrics.height == 64)
        #expect(MarketsRowMetrics.logo == 40)
        #expect(MarketsRowMetrics.sparklineWidth == 60)
    }

    @Test func theChipsAreAllWatchlistPreIpoAndHeld() {
        #expect(MonacoCore.StocksTabModel.availableBrowses.map(\.title) == ["All", "Watchlist", "Pre-IPO", "Held"])
        let count = MonacoCore.StocksTabModel.availableBrowses.count
        #expect(MonacoChipBar<MonacoCore.StocksTabModel.Browse>.fits(count, maxVisible: 4))
    }
}
