#if DEBUG
import Foundation
import MonacoAPI

extension BestPrice {
    public static func sampleComparison(now: Date) -> Components.Schemas.PriceComparison {
        func quote(
            _ id: String, issuer: String, network: Components.Schemas.Network,
            venue: Components.Schemas.RouteQuote.VenuePayload, shares: String
        ) -> Components.Schemas.RouteQuote {
            Components.Schemas.RouteQuote(
                listingId: id, issuer: issuer, network: network, venue: venue, netShareNanos: shares,
                netMicros: "10000000", costs: [.init(kind: .poolFee, micros: "3000")], quotedAt: now,
                expiresAt: now.addingTimeInterval(30))
        }
        return Components.Schemas.PriceComparison(
            winnerListingId: "01928a6e-0000-7000-8000-000000000002",
            quotes: [
                quote(
                    "01928a6e-0000-7000-8000-000000000002", issuer: "xstocks", network: .base, venue: .uniswap,
                    shares: "40120000"),
                quote(
                    "01928a6e-0000-7000-8000-000000000001", issuer: "xstocks", network: .solana, venue: .jupiter,
                    shares: "40000000"),
                quote(
                    "01928a6e-0000-7000-8000-000000000003", issuer: "ondo", network: .ethereum, venue: .uniswap,
                    shares: "39800000"),
            ])
    }

    public static func sample(now: Date) -> BestPrice? { BestPrice(sampleComparison(now: now), isSell: false) }
}
#endif
