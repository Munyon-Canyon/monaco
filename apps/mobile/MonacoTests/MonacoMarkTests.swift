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

struct MarkGeometryTests {
    @Test func radiusIsThirtyTwoPercentOfSize() {
        #expect(abs(MarkGeometry.radius(for: 44) - 14.08) < 0.001)
        #expect(abs(MarkGeometry.radius(for: 54) - 17.28) < 0.001)
    }

    @Test func stockMarkSizesMatchTheListedValues() {
        #expect(
            [StockMarkSize.feed, .mover, .card, .row, .header, .review, .receipt].map(\.rawValue)
                == [28, 31, 35, 42, 47, 48, 54])
    }

    @Test func avatarSizesMatchTheListedValues() {
        let sizes: [MonacoAvatarSize] = [
            .author, .chat, .comment, .feed, .preview, .rank, .activity, .row, .identity, .edit, .large,
        ]
        #expect(sizes.map(\.rawValue) == [24, 26, 30, 32, 34, 36, 38, 40, 64, 76, 80])
    }
}

struct MonacoIconTests {
    @Test(arguments: MonacoIcon.allCases)
    func everyIconResolvesToASystemSymbol(icon: MonacoIcon) {
        #expect(UIImage(systemName: icon.symbol) != nil)
        #expect(UIImage(systemName: icon.filled) != nil)
    }
}

struct MonacoFaceStackTests {
    @Test func spokenLabelNamesThreeAndCountsTheRest() {
        let label = MonacoFaceStack.spokenLabel(names: ["Ana", "Ben", "Cy", "Dee", "Eli"], total: 7)
        #expect(label == "Ana, Ben, Cy and 4 others")
    }

    @Test func spokenLabelWithFewMembersHasNoTail() {
        #expect(MonacoFaceStack.spokenLabel(names: ["Ana", "Ben"], total: 2) == "Ana, Ben")
    }
}

@MainActor
struct CabalCoverTests {
    private func size(of style: CabalCover<Color>.Style, url: URL?) -> CGSize {
        let cover = CabalCover(cabalId: "cabal-1", tint: .pine, pictureURL: url, style: style) { Color.clear }
        let host = UIHostingController(rootView: cover)
        return host.sizeThatFits(in: CGSize(width: 300, height: 600))
    }

    @Test func aPictureURLDoesNotChangeTheLayout() {
        let stub = URL(string: "https://example.invalid/cover.jpg")
        for style: CabalCover<Color>.Style in [.hero, .heroTall, .thumb(34)] {
            #expect(size(of: style, url: nil) == size(of: style, url: stub))
        }
    }

    @Test func heroIsAtLeastItsMinimumHeight() {
        #expect(size(of: .hero, url: nil).height >= 132)
        #expect(size(of: .heroTall, url: nil).height >= 172)
        #expect(size(of: .thumb(34), url: nil) == CGSize(width: 34, height: 34))
    }

    @Test func thumbHasNoScrimAndReduceTransparencyPicksTheSolidOne() {
        #expect(CabalCover<EmptyView>.scrim(for: .thumb(34), reduceTransparency: true) == nil)
        #expect(CabalCover<EmptyView>.scrim(for: .hero, reduceTransparency: false) == .hero)
        #expect(CabalCover<EmptyView>.scrim(for: .card, reduceTransparency: false) == .card)
        #expect(CabalCover<EmptyView>.scrim(for: .memberCard, reduceTransparency: false) == .memberCard)
        for style: CabalCover<EmptyView>.Style in [.hero, .heroTall, .card, .memberCard] {
            #expect(CabalCover<EmptyView>.scrim(for: style, reduceTransparency: true) == .solid)
        }
    }
}
