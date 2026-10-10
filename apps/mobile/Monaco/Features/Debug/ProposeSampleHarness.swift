#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI

enum ProposeSampleScenario: String, CaseIterable {
    case chooser
    case chooserCashOnly
    case buyList
    case buyListPaused
    case amount
    case potFailed
    case pickCabal
    case noCabal
    case sell
    case review
    case sellReview

    static let launchArgument = "-MonacoProposeSample"

    static func matching(_ arguments: [String]) -> ProposeSampleScenario? {
        guard let flag = arguments.firstIndex(of: launchArgument), arguments.indices.contains(flag + 1) else {
            return nil
        }
        return ProposeSampleScenario(rawValue: arguments[flag + 1])
    }

    var script: SampleAPIScript {
        var script = SampleAPIScript()
        switch self {
        case .chooserCashOnly: script.pot = .sampleCashOnly
        case .potFailed: script.mode = .potUnavailable
        case .pickCabal: script.cabalCount = 2
        case .noCabal: script.cabalCount = 0
        case .buyListPaused: script.includesPausedStock = true
        default: break
        }
        return script
    }

    var tab: MainTab {
        switch self {
        case .amount, .potFailed, .pickCabal, .noCabal, .sell: .stocks
        default: .cabals
        }
    }

    var route: any AppRoute {
        switch self {
        case .chooser, .chooserCashOnly: ProposeRoute(cabalID: GroupDetailSampleData.cabalID)
        case .buyList, .buyListPaused: SampleScreenRoute(screen: .proposeBuy, arguments: [])
        case .amount, .potFailed, .pickCabal, .noCabal: ProposeFromAssetRoute(symbol: "GOOGLx", kind: .buy)
        case .sell: ProposeFromAssetRoute(symbol: "GOOGLx", kind: .sell)
        case .review: SampleScreenRoute(screen: .proposeReview(.buy), arguments: [])
        case .sellReview: SampleScreenRoute(screen: .proposeReview(.sell), arguments: [])
        }
    }
}

final class ProposeSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        guard let scenario = ProposeSampleScenario.matching(arguments) else { return nil }
        SampleAPIProtocol.install(scenario.script)
        return AnyView(SampleAppFrame(auth: auth, tab: scenario.tab, routes: [scenario.route]))
    }
}

struct ProposeReviewSample: View {
    let kind: ProposeKind
    @Environment(AppEnvironment.self) private var environment
    @State private var memory = MonacoCore.ProposeReviewMemory()

    var body: some View {
        ProposeReviewScreen(
            service: MonacoCore.LiveProposeService(api: environment.api), cabalID: GroupDetailSampleData.cabalID,
            draft: draft, preview: MonacoCore.ProposePreview(.proposalPreviewClean), trade: trade,
            buyName: kind == .buy ? "Alphabet" : nil, memory: memory)
    }

    private var holding: ProposeHolding {
        ProposeHolding(Components.Schemas.CabalPot.sampleInvested.holdings[0])
    }

    private var trade: MonacoCore.ProposeTrade {
        kind == .buy ? .buy(symbol: "GOOGLx", kind: .stock, tokenDecimals: nil) : .sell(holding)
    }

    private var draft: MonacoCore.ProposalDraft {
        let thesis = "Earnings next week."
        return kind == .buy
            ? .buy(symbol: "GOOGLx", usdcMicros: 25_000_000, thesis: thesis)
            : .sell(symbol: "GOOGLx", tokenAmount: holding.tokenAmount(forMicros: 25_000_000), thesis: thesis)
    }
}
#endif
