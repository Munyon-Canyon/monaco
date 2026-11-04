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

    @Test func cancelClearsTheQueryAndLeavesSearch() {
        var query = "micro"
        var searching = true
        var focused = true
        MarketsSearchCancel.apply(query: &query, searching: &searching, focused: &focused)
        #expect(MarketsSearchCancel.identifier == "markets-search-cancel")
        #expect(query == "")
        #expect(!searching)
        #expect(!focused)
    }

    @Test func aPreIpoTileShowsTheFirstWordOfTheName() {
        #expect(StockMark.tileText(symbol: "SPACEX", displayName: "SpaceX Holdings", assetKind: .preIpo) == "SpaceX")
        #expect(StockMark.tileText(symbol: "OPENAI", displayName: "OpenAI", assetKind: .preIpo) == "OpenAI")
    }

    @Test func aPreIpoTileWithoutANameFallsBackToTheTicker() {
        let symbol = "ANTHROPIC"
        let expected = StockMark.tileText(forTicker: AssetSymbolFormatter.display(symbol, kind: .preIpo))
        #expect(StockMark.tileText(symbol: symbol, displayName: nil, assetKind: .preIpo) == expected)
        #expect(StockMark.tileText(symbol: symbol, displayName: "  ", assetKind: .preIpo) == expected)
    }

    @Test func aStockTileKeepsItsTickerWhateverItsName() {
        #expect(StockMark.tileText(symbol: "MU", displayName: "Micron Technology", assetKind: .stock) == "MU")
        #expect(StockMark.tileText(symbol: "SMCI", displayName: "Super Micro Computer", assetKind: .stock) == "SMCI")
    }
}
