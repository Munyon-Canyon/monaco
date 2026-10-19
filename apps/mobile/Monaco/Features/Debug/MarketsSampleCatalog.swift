#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI

enum MarketsSampleCatalog {
    static let userID = Components.Schemas.Me.sample.id

    static let assets: [Components.Schemas.AssetSummary] = [
        listing("NVDAx", "NVIDIA xStock", 220_000_000, 418),
        listing("AAPLx", "Apple xStock", 240_000_000, 124),
        listing("TSLAx", "Tesla xStock", 262_500_000, -240),
        listing("MSFTx", "Microsoft xStock", 350_000_000, 62),
        listing("AMZNx", "Amazon xStock", 205_000_000, -35),
        listing("GOOGLx", "Alphabet xStock", 180_000_000, -41),
        listing("METAx", "Meta xStock", 585_000_000, 88),
        listing("AMDx", "AMD xStock", 178_400_000, 305),
    ]

    static let watched = ["AAPLx", "NVDAx", "MSFTx"]

    @MainActor private static var installed = false

    @MainActor
    static func install(watched symbols: [String]) {
        guard !installed else { return }
        installed = true
        WatchlistModels.shared = WatchlistModels(store: InMemoryWatchlistStore([userID: symbols]))
    }

    private static func listing(
        _ symbol: String, _ name: String, _ priceMicros: Int64, _ changeBps: Int32
    ) -> Components.Schemas.AssetSummary {
        Components.Schemas.AssetSummary(
            symbol: symbol, displayName: name, issuer: .xstocks, kind: .equity, logoUrl: nil,
            priceMicros: priceMicros, priceAsOf: Date(), changeBps: changeBps,
            sparklineMicros: [priceMicros - 1_000_000, priceMicros], quotable: true, session: .open, tradable: true)
    }
}

nonisolated extension Components.Schemas.AssetDetail {
    init(listing: Components.Schemas.AssetSummary) {
        self.init(
            symbol: listing.symbol, displayName: listing.displayName, issuer: listing.issuer, kind: listing.kind,
            logoUrl: listing.logoUrl, priceMicros: listing.priceMicros, priceAsOf: listing.priceAsOf,
            changeBps: listing.changeBps, sparklineMicros: listing.sparklineMicros, quotable: listing.quotable,
            session: listing.session, decimals: 8, uiMultiplier: .init(num: 1, den: 1),
            tradable: listing.tradable ?? true, otherListings: [], attribution: "Data provided by CoinGecko")
    }
}
#endif
