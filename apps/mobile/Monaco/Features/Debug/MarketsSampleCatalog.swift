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

    static let preIpo: [Components.Schemas.AssetSummary] = [
        preIpoListing("tSpaceX", "SpaceX", 185_000_000, eligible: true),
        preIpoListing("tOpenAI", "OpenAI", 148_000_000, eligible: true),
        preIpoListing("tAnthropic", "Anthropic", 96_500_000, eligible: false),
        preIpoListing("tStripe", "Stripe", 52_300_000, eligible: true),
        preIpoListing("tDatabricks", "Databricks", 78_200_000, eligible: false),
    ]

    static let marketsAssets = assets + preIpo

    static let searchAssets: [Components.Schemas.AssetSummary] = [
        searchListing("MSFTx", "Microsoft xStock", 512_300_000, 60),
        searchListing("AMDx", "AMD Advanced Micro Devices xStock", 162_300_000, 790),
        searchListing("MUx", "Micron xStock", 188_400_000, 190),
        searchListing("SMCIx", "Supermicro xStock", 44_100_000, 260),
        searchListing("MCHPx", "Microchip xStock", 71_200_000, -70),
    ]

    static let watched = ["AAPLx", "NVDAx", "MSFTx"]

    static let marketsWatched = ["NVDAx", "AAPLx", "TSLAx", "METAx", "AMZNx", "MSFTx", "tSpaceX"]

    @MainActor private static var installed = false

    @MainActor
    static func install(watched symbols: [String]) {
        guard !installed else { return }
        installed = true
        WatchlistModels.shared = WatchlistModels(store: InMemoryWatchlistStore([userID: symbols]))
    }

    private static func preIpoListing(
        _ symbol: String, _ name: String, _ estimateMicros: Int64, eligible: Bool
    ) -> Components.Schemas.AssetSummary {
        Components.Schemas.AssetSummary(
            id: "01890a5d-ac96-774b-bcce-b302099a8070",
            symbol: symbol, displayName: name, issuer: .tessera, kind: .preIpo, logoUrl: nil,
            priceMicros: estimateMicros / 4, priceAsOf: Date(), changeBps: 125,
            sparklineMicros: [estimateMicros / 4 - 1_000_000, estimateMicros / 4], quotable: true,
            session: .init(state: .open, continuous: true, holiday: "", earlyClose: false), tradable: eligible,
            estimate: .init(priceMicros: estimateMicros, asOf: "2026-10-20"))
    }

    private static func searchListing(
        _ symbol: String, _ name: String, _ priceMicros: Int64, _ changeBps: Int32
    ) -> Components.Schemas.AssetSummary {
        let step = priceMicros / 200
        let slope: Int64 = changeBps < 0 ? -1 : 1
        let wobble: [Int64] = [0, 3, 1, 4, 2, 5, 4, 7, 6, 9, 8, 11]
        let series = wobble.map { priceMicros - slope * (11 - $0) * step }
        return Components.Schemas.AssetSummary(
            id: "01890a5d-ac96-774b-bcce-b302099a8070",
            symbol: symbol, displayName: name, issuer: .xstocks, kind: .equity, logoUrl: nil,
            priceMicros: priceMicros, priceAsOf: Date(), changeBps: changeBps, sparklineMicros: series,
            quotable: true, session: .open, tradable: true)
    }

    private static func listing(
        _ symbol: String, _ name: String, _ priceMicros: Int64, _ changeBps: Int32
    ) -> Components.Schemas.AssetSummary {
        Components.Schemas.AssetSummary(
            id: "01890a5d-ac96-774b-bcce-b302099a8070",
            symbol: symbol, displayName: name, issuer: .xstocks, kind: .equity, logoUrl: nil,
            priceMicros: priceMicros, priceAsOf: Date(), changeBps: changeBps,
            sparklineMicros: [priceMicros - 1_000_000, priceMicros], quotable: true, session: .open, tradable: true)
    }
}

nonisolated extension Components.Schemas.AssetDetail {
    init(listing: Components.Schemas.AssetSummary) {
        self.init(
            id: listing.id, symbol: listing.symbol,
            displayName: listing.displayName, issuer: listing.issuer, kind: listing.kind,
            logoUrl: listing.logoUrl, priceMicros: listing.priceMicros, priceAsOf: listing.priceAsOf,
            changeBps: listing.changeBps, sparklineMicros: listing.sparklineMicros, quotable: listing.quotable,
            session: listing.session, decimals: 8, uiMultiplier: .init(num: 1, den: 1),
            tradable: listing.tradable ?? true, otherListings: [], attribution: "Data provided by CoinGecko",
            pricePollIntervalSeconds: 120,
            preIpo: listing.estimate.map {
                .init(
                    estimate: .init(priceMicros: $0.priceMicros, asOf: $0.asOf), valuationMicros: "0",
                    lastRound: nil, rounds: [], minOrderMicros: 10_000_000, sellWindow: .tenderQuarterly,
                    nextTenderOn: nil, imageUrl: nil)
            })
    }
}
#endif
