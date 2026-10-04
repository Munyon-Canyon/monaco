import MonacoCore
import Testing

@testable import Monaco

@MainActor
struct ProposeBuyStockTests {
    @Test func anUntradableStockDoesNotAdvanceWhenTapped() {
        var selected: ProposeStock?
        ProposeBuyStockSelection.select(asset(tradable: false)) { selected = $0 }
        #expect(selected == nil)
    }

    private func asset(tradable: Bool) -> MarketAsset {
        MarketAsset(
            symbol: "AMBRx", ticker: "AMBR", name: "Amber", issuer: .xstocks, kind: .stock, logoURL: nil,
            priceMicros: 25_000_000, priceText: "$25.00", changeBasisPoints: nil, changeText: "—", sparkline: nil,
            session: .open, status: .init(session: .open, isOpen: true, afterHours: false), showsSessionChip: false,
            isTradable: tradable
        )
    }
}
