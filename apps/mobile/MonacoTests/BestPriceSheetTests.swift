import Foundation
import MonacoAPI
import MonacoCore
import Testing

@testable import Monaco

@MainActor
struct BestPriceSheetTests {
    private let now = Date(timeIntervalSince1970: 1_791_100_800)

    private func bestPrice(isSell: Bool = false) throws -> BestPrice {
        try #require(BestPrice(BestPrice.sampleComparison(now: now), isSell: isSell))
    }

    @Test func theProposalRowShowsTheWinningPriceAndHowManyVenuesWereChecked() throws {
        let best = try bestPrice()
        #expect(BestPriceSection.figure(best) == "$249.25 a share")
        #expect(best.headline == "Best price across 3 venues")
    }

    @Test func theSheetListsEveryVenueCheckedWithTheWinnerFirst() throws {
        let rows = BestPriceSheet.rows(try bestPrice())
        #expect(rows.map(\.name) == ["Base", "Solana", "Ethereum", "Monad"])
        #expect(rows.map(\.figure) == ["0.0401 shares", "0.0400 shares", "0.0398 shares", "No price yet"])
        #expect(rows.map(\.isWinner) == [true, false, false, false])
        #expect(try bestPrice().advantage == "You get 0.0001 more shares than the next best")
    }

    @Test func aSellListsDollarsReceived() throws {
        let rows = BestPriceSheet.rows(try bestPrice(isSell: true))
        #expect(rows.map(\.figure) == ["$10.00", "$10.00", "$10.00"])
    }
}
