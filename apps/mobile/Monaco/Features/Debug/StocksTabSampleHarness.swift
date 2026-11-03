#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI

enum StocksTabSampleScenario: String, CaseIterable {
    case ready
    case watchlistEmpty
    case noResults
    case search
    case loading
    case stale
    case failed
    case empty
    case paused
    case held
    case movers
    case listMovers

    static let launchArgument = "-MonacoStocksTabSample"

    var script: SampleAPIScript {
        var script = SampleAPIScript(catalog: MarketsSampleCatalog.marketsAssets)
        switch self {
        case .ready, .watchlistEmpty, .noResults, .stale: break
        case .held: script.held = .sampleAndres
        case .search: script = SampleAPIScript(catalog: MarketsSampleCatalog.searchAssets)
        case .loading: script.mode = .hang
        case .failed: script.mode = .assetsUnavailable
        case .empty: script.mode = .empty
        case .paused: script = SampleAPIScript(includesPausedStock: true)
        case .movers, .listMovers: script = SampleAPIScript(catalog: MoversSample.catalog)
        }
        return script
    }

    var watched: [String] {
        switch self {
        case .ready, .stale, .movers: MarketsSampleCatalog.marketsWatched
        case .watchlistEmpty, .noResults, .search, .loading, .failed, .empty, .paused, .held, .listMovers: []
        }
    }

    var query: String {
        switch self {
        case .noResults: "zzzz"
        case .search: "micro"
        default: ""
        }
    }

    var browse: MonacoCore.StocksTabModel.Browse { self == .held ? .held : .all }

    var routes: [any AppRoute] {
        self == .listMovers ? [MarketsListRoute(order: .movers)] : []
    }

    var hints: any HintConnecting {
        self == .stale ? SampleTickHints(after: .seconds(2)) : SampleSilentHints()
    }
}

enum MoversSample {
    static let movers: [Components.Schemas.AssetSummary] = [
        mover("AMDx", "AMD xStock", 178_400_000, 790),
        mover("NFLXx", "Netflix xStock", 1_180_000_000, 520),
        mover("TSLAx", "Tesla xStock", 262_500_000, -240),
        mover("AMZNx", "Amazon xStock", 205_000_000, -210),
        mover("MSFTx", "Microsoft xStock", 350_000_000, 160),
    ]

    static let catalog =
        movers + MarketsSampleCatalog.marketsAssets.filter { asset in !movers.contains { $0.symbol == asset.symbol } }

    private static func mover(
        _ symbol: String, _ name: String, _ priceMicros: Int64, _ changeBps: Int32
    ) -> Components.Schemas.AssetSummary {
        let step = priceMicros / 200
        let slope: Int64 = changeBps < 0 ? -1 : 1
        let wobble: [Int64] = [0, 3, 1, 4, 2, 5, 4, 7, 6, 9, 8, 11]
        return Components.Schemas.AssetSummary(
            id: "01890a5d-ac96-774b-bcce-b302099a8070",
            symbol: symbol, displayName: name, issuer: .xstocks, kind: .equity, logoUrl: nil,
            priceMicros: priceMicros, priceAsOf: Date(), changeBps: changeBps,
            sparklineMicros: wobble.map { priceMicros - slope * (11 - $0) * step }, quotable: true,
            session: .open, tradable: true)
    }
}

nonisolated struct SampleTickHints: HintConnecting {
    let after: Duration

    func hints(matching filter: HintFilter) -> AsyncStream<Hint> {
        let hint = Hint.changed(.global, what: "prices_updated", id: "sample")
        return AsyncStream { continuation in
            let tick = Task {
                try? await Task.sleep(for: after)
                if filter.matches(hint) {
                    SampleAPIProtocol.failAssetReads()
                    continuation.yield(hint)
                }
            }
            continuation.onTermination = { _ in tick.cancel() }
        }
    }

    func start() async {}
    func stop() async {}
}

final class StocksTabSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        return scenarioRoot(
            StocksTabSampleScenario.self, flag: StocksTabSampleScenario.launchArgument, arguments: arguments
        ) { scenario in
            SampleAPIProtocol.install(scenario.script)
            MarketsSampleCatalog.install(watched: scenario.watched)
            StocksTab.sampleQuery = scenario.query
            StocksTabView.sampleBrowse = scenario.browse
            return AnyView(SampleAppFrame(auth: auth, tab: .stocks, routes: scenario.routes, hints: scenario.hints))
        }
    }
}
#endif
