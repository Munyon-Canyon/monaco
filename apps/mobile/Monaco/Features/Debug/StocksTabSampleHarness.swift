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

    static let launchArgument = "-MonacoStocksTabSample"

    static func matching(_ arguments: [String]) -> StocksTabSampleScenario? {
        guard let flag = arguments.firstIndex(of: launchArgument), arguments.indices.contains(flag + 1) else {
            return nil
        }
        return StocksTabSampleScenario(rawValue: arguments[flag + 1])
    }

    var script: SampleAPIScript {
        var script = SampleAPIScript(catalog: MarketsSampleCatalog.marketsAssets)
        switch self {
        case .ready, .watchlistEmpty, .noResults: break
        case .search: script = SampleAPIScript(catalog: MarketsSampleCatalog.searchAssets)
        case .loading: script.mode = .hang
        case .stale: script.failsAfterAssetReads = 1 + StocksTabSampleScenario.watchedCount
        case .failed: script.mode = .assetsUnavailable
        case .empty: script.mode = .empty
        case .paused: script = SampleAPIScript(includesPausedStock: true)
        }
        return script
    }

    var watched: [String] {
        switch self {
        case .ready, .stale: MarketsSampleCatalog.marketsWatched
        case .watchlistEmpty, .noResults, .search, .loading, .failed, .empty, .paused: []
        }
    }

    var query: String {
        switch self {
        case .noResults: "zzzz"
        case .search: "micro"
        default: ""
        }
    }

    var hints: any HintConnecting {
        self == .stale ? SampleTickHints(after: .seconds(2)) : SampleSilentHints()
    }

    private static let watchedCount = MarketsSampleCatalog.marketsWatched.count
}

nonisolated struct SampleTickHints: HintConnecting {
    let after: Duration

    func hints(matching filter: HintFilter) -> AsyncStream<Hint> {
        let hint = Hint.changed(.global, what: "prices_updated", id: "sample")
        return AsyncStream { continuation in
            let tick = Task {
                try? await Task.sleep(for: after)
                if filter.matches(hint) { continuation.yield(hint) }
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
        guard let scenario = StocksTabSampleScenario.matching(arguments) else { return nil }
        SampleAPIProtocol.install(scenario.script)
        MarketsSampleCatalog.install(watched: scenario.watched)
        StocksTab.sampleQuery = scenario.query
        return AnyView(SampleAppFrame(auth: auth, tab: .stocks, hints: scenario.hints))
    }
}
#endif
