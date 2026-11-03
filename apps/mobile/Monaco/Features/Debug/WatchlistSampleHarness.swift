#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI

enum WatchlistSampleScenario: String, CaseIterable {
    case ready
    case empty
    case noResults
    case loading
    case failed

    static let launchArgument = "-MonacoWatchlistSample"

    var script: SampleAPIScript {
        var script = SampleAPIScript(catalog: MarketsSampleCatalog.assets)
        switch self {
        case .loading: script.mode = .hang
        case .failed: script.mode = .assetsUnavailable
        case .ready, .empty, .noResults: break
        }
        return script
    }

    var watched: [String] { self == .ready ? MarketsSampleCatalog.watched : [] }

    var query: String { self == .noResults ? "zzzz" : "" }
}

final class WatchlistSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        return scenarioRoot(
            WatchlistSampleScenario.self, flag: WatchlistSampleScenario.launchArgument, arguments: arguments
        ) { scenario in
            SampleAPIProtocol.install(scenario.script)
            MarketsSampleCatalog.install(watched: scenario.watched)
            return AnyView(
                SampleAppFrame(auth: auth, tab: .stocks, routes: [WatchlistRoute(query: scenario.query)]))
        }
    }
}
#endif
