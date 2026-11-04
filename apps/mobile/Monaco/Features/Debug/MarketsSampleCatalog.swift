#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI

enum MarketsSampleCatalog {
    static let userID = Components.Schemas.Me.sample.id

    static let assets: [Components.Schemas.AssetSummary] = [
        listing("NVDAx", "NVIDIA xStock", 220_000_000, 418),
        listing("AAPLx", "Apple xStock", 180_000_000, -80),
        listing("TSLAx", "Tesla xStock", 412_550_000, -240),
        listing("MSFTx", "Microsoft xStock", 512_300_000, 60),
        listing("AMZNx", "Amazon xStock", 184_500_000, -210),
        listing("GOOGLx", "Alphabet xStock", 180_000_000, -41),
        listing("METAx", "Meta xStock", 724_400_000, 110),
        listing("AMDx", "AMD xStock", 178_400_000, 305),
    ]

    static let preIpo: [Components.Schemas.AssetSummary] = [
        preIpoListing(
            "tSpaceX", "SpaceX", 185_000_000, asOf: "2026-10-20", eligible: true,
            valuationMicros: "350000000000000000", lastRoundOn: "2026-06-01"),
        preIpoListing(
            "tOpenAI", "OpenAI", 310_000_000, asOf: "2026-10-20", eligible: true,
            valuationMicros: "500000000000000000", lastRoundOn: "2026-03-01"),
        preIpoListing(
            "tAnthropic", "Anthropic", 96_400_000, asOf: "2026-10-18", eligible: false,
            valuationMicros: "183000000000000000"),
        preIpoListing("tStripe", "Stripe", 27_500_000, asOf: "2026-10-14", eligible: true),
        preIpoListing("tDatabricks", "Databricks", 71_800_000, asOf: "2026-10-09", eligible: false),
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
        _ symbol: String, _ name: String, _ estimateMicros: Int64, asOf: String, eligible: Bool,
        valuationMicros: String? = nil, lastRoundOn: String? = nil
    ) -> Components.Schemas.AssetSummary {
        Components.Schemas.AssetSummary(
            id: "01890a5d-ac96-774b-bcce-b302099a8070",
            symbol: symbol, displayName: name, issuer: .tessera, kind: .preIpo, logoUrl: nil,
            priceMicros: estimateMicros / 4, priceAsOf: Date(), changeBps: 125,
            sparklineMicros: [estimateMicros / 4 - 1_000_000, estimateMicros / 4], quotable: true,
            session: .init(state: .open, continuous: true, holiday: "", earlyClose: false), tradable: eligible,
            estimate: .init(priceMicros: estimateMicros, asOf: asOf),
            valuationMicros: valuationMicros, lastRoundOn: lastRoundOn)
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

    private static func trend(_ priceMicros: Int64, _ changeBps: Int32) -> [Int64] {
        let step = priceMicros / 200
        let slope: Int64 = changeBps < 0 ? -1 : 1
        let wobble: [Int64] = [0, 3, 1, 4, 2, 5, 4, 7, 6, 9, 8, 11]
        return wobble.map { priceMicros - slope * (11 - $0) * step }
    }

    private static func listing(
        _ symbol: String, _ name: String, _ priceMicros: Int64, _ changeBps: Int32
    ) -> Components.Schemas.AssetSummary {
        Components.Schemas.AssetSummary(
            id: "01890a5d-ac96-774b-bcce-b302099a8070",
            symbol: symbol, displayName: name, issuer: .xstocks, kind: .equity, logoUrl: nil,
            priceMicros: priceMicros, priceAsOf: Date(), changeBps: changeBps,
            sparklineMicros: trend(priceMicros, changeBps), quotable: true, session: .open, tradable: true)
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
