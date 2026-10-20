import Foundation
import MonacoAPI
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

    @Test func theStockPickedFromTheListKeepsItsLogo() {
        let logo = URL(string: "https://logos.example/AMBRx.png")
        var selected: ProposeStock?
        ProposeBuyStockSelection.select(asset(tradable: true, logoURL: logo)) { selected = $0 }
        #expect(selected?.logoURL == logo)
    }

    @Test func aStockPickedWithoutALogoHasNone() {
        var selected: ProposeStock?
        ProposeBuyStockSelection.select(asset(tradable: true)) { selected = $0 }
        #expect(selected != nil)
        #expect(selected?.logoURL == nil)
    }

    @Test func theStockFromTheStockScreenKeepsTheResolvedLogo() {
        let logo = URL(string: "https://logos.example/GOOGLx.png")
        #expect(ProposeStock(symbol: "GOOGLx", logoURL: logo).logoURL == logo)
        #expect(ProposeStock(symbol: "GOOGLx").logoURL == nil)
    }

    @Test func aSellStockKeepsTheLogoOfItsHolding() {
        let logo = URL(string: "https://logos.example/GOOGLx.png")
        let holding = ProposeHolding(Components.Schemas.CabalPot.sampleInvested.holdings[0])
        #expect(ProposeStock(holding: holding, logoURL: logo).logoURL == logo)
        #expect(ProposeStock(holding: holding).logoURL == nil)
    }

    private func asset(tradable: Bool, logoURL: URL? = nil) -> MarketAsset {
        MarketAsset(
            symbol: "AMBRx", ticker: "AMBR", name: "Amber", issuer: .xstocks, kind: .stock, logoURL: logoURL,
            priceMicros: 25_000_000, priceText: "$25.00", changeBasisPoints: nil, changeText: "—", sparkline: nil,
            session: .open, status: .init(session: .open, isOpen: true, afterHours: false), showsSessionChip: false,
            isTradable: tradable
        )
    }
}
