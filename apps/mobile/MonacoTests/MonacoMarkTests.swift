import SwiftUI
import Testing
import UIKit

@testable import Monaco

struct StockMarkTests {
    @Test(arguments: [
        ("AAPL", "AAPL"),
        ("aapl", "AAPL"),
        ("AMZN", "AMZN"),
        ("AVGO", "AVGO"),
        ("ABBV", "ABBV"),
        ("V", "V"),
        ("GOOGL", "GOOGL"),
        ("ABCDEF", "ABCDE"),
        (" nvda ", "NVDA"),
        ("", ""),
        ("BRK.B", "BRK.B"),
        ("brk.b", "BRK.B"),
        ("ABCD.EF", "ABCD"),
    ])
    func tileTextKeepsTheWholeTicker(ticker: String, expected: String) {
        #expect(StockMark.tileText(forTicker: ticker) == expected)
    }

    @Test func aSeparatorThatIsNotAtTheEndIsKept() {
        #expect(StockMark.tileText(forTicker: "BF.B") == "BF.B")
    }

    @Test func tickersStartingWithTheSameLetterGetDifferentTiles() {
        let tickers = ["AAPL", "ABBV", "ABT", "ACN", "AMBR", "AMZN", "APP", "AVGO", "AZN"]
        let tiles = Set(tickers.map { StockMark.tileText(forTicker: $0) })
        #expect(tiles.count == tickers.count)
    }

    @Test func longerTickersAreSetSmaller() {
        #expect(StockMark.textScale(for: "A") > StockMark.textScale(for: "AA"))
        #expect(StockMark.textScale(for: "AAP") > StockMark.textScale(for: "AAPL"))
    }
}
