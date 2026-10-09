#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI

enum AssetDetailSampleScenario: String, CaseIterable {
    case open
    case notHeld
    case preMarket
    case afterHours
    case closed
    case preIpo
    case untradable
    case paused
    case emptyChart
    case chartFailed
    case loading
    case failed

    static let launchArgument = "-MonacoAssetDetailSample"

    static func matching(_ arguments: [String]) -> AssetDetailSampleScenario? {
        guard let flag = arguments.firstIndex(of: launchArgument), arguments.indices.contains(flag + 1) else {
            return nil
        }
        return AssetDetailSampleScenario(rawValue: arguments[flag + 1])
    }

    var script: SampleAPIScript {
        var script = SampleAPIScript()
        switch self {
        case .open: break
        case .notHeld: script.cabalCount = 0
        case .preMarket: script.asset.session = .preMarket
        case .afterHours: script.asset.session = .afterHours
        case .closed:
            script.asset.session = .init(
                state: .closed, continuous: false, holiday: "Thanksgiving Day", earlyClose: false)
        case .preIpo: script.asset = .spaceX
        case .untradable: script.asset.tradable = false
        case .paused: script.asset = .paused
        case .emptyChart: script.chart = .empty
        case .chartFailed: script.mode = .chartUnavailable
        case .loading: script.mode = .hang
        case .failed: script.mode = .assetsUnavailable
        }
        return script
    }
}

final class AssetDetailSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        guard let scenario = AssetDetailSampleScenario.matching(arguments) else { return nil }
        let script = scenario.script
        SampleAPIProtocol.install(script)
        return AnyView(SampleAppFrame(auth: auth, tab: .stocks, routes: [AssetRoute(symbol: script.asset.symbol)]))
    }
}
#endif
