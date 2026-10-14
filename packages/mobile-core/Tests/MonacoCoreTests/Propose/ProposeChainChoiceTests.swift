import Foundation
import MonacoAPI
import MonacoCore
import XCTest

final class ProposeChainChoiceTests: XCTestCase {
    func testOptionsLeadWithBestPriceThenEachChainsBestListing() {
        let options = ProposeChainOption.options(Self.bestPrice(Self.fourVenues))

        XCTAssertEqual(options.map(\.title), ["Best price", "Base", "Solana", "Ethereum", "Monad"])
        XCTAssertEqual(
            options.map(\.chain), [.best, .network(.base), .network(.solana), .network(.ethereum), .network(.monad)])
        XCTAssertEqual(
            options.map(\.figure), ["0.4012 shares", "0.4012 shares", "0.4000 shares", "0.3980 shares", "No price yet"])
        XCTAssertEqual(options.map(\.network), [.base, .base, .solana, .ethereum, .monad])
        XCTAssertEqual(options.map(\.isBest), [true, false, false, false, false])
    }

    func testNoOptionsForASellNoComparisonOrOneChain() {
        XCTAssertEqual(ProposeChainOption.options(Self.bestPrice(Self.fourVenues, isSell: true)), [])
        XCTAssertEqual(ProposeChainOption.options(nil), [])
        let solanaOnly = [
            Self.quote("s1", .solana, shares: "400000000"), Self.quote("s2", .solana, shares: "390000000"),
        ]
        XCTAssertEqual(ProposeChainOption.options(Self.bestPrice(solanaOnly)), [])
    }

    static let fourVenues = [
        quote("base-a", .base, shares: "401200000"),
        quote("solana", .solana, shares: "400000000"),
        quote("ethereum", .ethereum, shares: "398000000"),
        quote("base-b", .base, shares: "390000000"),
    ]

    static func bestPrice(_ quotes: [Components.Schemas.RouteQuote], isSell: Bool = false) -> BestPrice? {
        BestPrice(
            Components.Schemas.PriceComparison(
                winnerListingId: quotes[0].listingId, quotes: quotes, unavailable: []),
            isSell: isSell)
    }

    static func quote(
        _ id: String, _ network: Components.Schemas.Network, shares: String
    ) -> Components.Schemas.RouteQuote {
        let now = Date()
        return Components.Schemas.RouteQuote(
            listingId: id, issuer: "xstocks", network: network, venue: network == .solana ? .jupiter : .uniswap,
            netShareNanos: shares, netMicros: "100000000", costs: [], quotedAt: now,
            expiresAt: now.addingTimeInterval(30))
    }
}
