#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI

enum StocksTabSampleScenario: String, CaseIterable {
    case full
    case loading
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
        switch self {
        case .full: SampleAPIScript()
        case .loading: SampleAPIScript(mode: .hang)
        case .failed: SampleAPIScript(mode: .assetsUnavailable)
        case .empty: SampleAPIScript(mode: .empty)
        case .paused: SampleAPIScript(includesPausedStock: true)
        }
    }
}

final class StocksTabSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        guard let scenario = StocksTabSampleScenario.matching(arguments) else { return nil }
        SampleAPIProtocol.install(scenario.script)
        return AnyView(SampleAppFrame(auth: auth, tab: .stocks))
    }
}
#endif
