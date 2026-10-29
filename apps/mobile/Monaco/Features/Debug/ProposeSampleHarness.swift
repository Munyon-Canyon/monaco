#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI

enum ProposeSampleScenario: String, CaseIterable {
    case chooser
    case chooserCashOnly
    case chooserNoHoldings
    case chooserNotMember
    case chooserPushed
    case buyList
    case buyListPaused
    case buyLoading
    case buyError
    case buyNoResults
    case sellList
    case sellNoHoldings
    case amount
    case amountSell
    case amountSellBase
    case amountTyped
    case amountInsufficient
    case amountEmptyThesis
    case amountError
    case potFailed
    case pickCabal
    case noCabal
    case sell
    case review
    case sellReview
    case reviewLongThesis
    case reviewSubmitting
    case reviewSent
    case reviewError
    case reviewNvidia
    case reviewNvidiaTwoCabals
    case reviewSpaceX

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
        case .chooserCashOnly, .chooserNoHoldings, .sellNoHoldings: script.pot = .sampleCashOnly
        case .amountSellBase: script.pot = .sampleMultichain
        case .chooserNotMember: script.role = nil
        case .potFailed, .amountError: script.mode = .potUnavailable
        case .buyError: script.mode = .assetsUnavailable
        case .buyLoading: script.mode = .hang
        case .pickCabal: script.cabalCount = 2
        case .noCabal: script.cabalCount = 0
        case .buyListPaused: script.includesPausedStock = true
        default: break
        }
        return script
    }

    var tab: MainTab {
        switch self {
        case .amount, .amountSell, .amountSellBase, .amountTyped, .amountInsufficient, .amountEmptyThesis,
            .amountError, .potFailed, .pickCabal, .noCabal, .sell:
            .stocks
        default: .cabals
        }
    }

    var route: any AppRoute {
        switch self {
        case .chooser, .chooserCashOnly, .chooserNoHoldings, .chooserNotMember:
            ProposeSampleRoute(scenario: self)
        case .chooserPushed: ProposeRoute(cabalID: GroupDetailSampleData.cabalID)
        case .buyList, .buyListPaused, .buyLoading, .buyError:
            SampleScreenRoute(screen: .proposeBuy, arguments: [])
        case .buyNoResults: ProposeSampleRoute(scenario: self)
        case .sellList, .sellNoHoldings: ProposeRoute(cabalID: GroupDetailSampleData.cabalID, start: .sell)
        case .amount, .potFailed, .pickCabal, .noCabal: ProposeFromAssetRoute(symbol: "GOOGLx", kind: .buy)
        case .amountSell, .amountSellBase, .sell: ProposeFromAssetRoute(symbol: "GOOGLx", kind: .sell)
        case .amountTyped, .amountInsufficient, .amountEmptyThesis, .amountError: ProposeSampleRoute(scenario: self)
        case .review: SampleScreenRoute(screen: .proposeReview(.buy), arguments: [])
        case .sellReview: SampleScreenRoute(screen: .proposeReview(.sell), arguments: [])
        case .reviewLongThesis, .reviewSubmitting, .reviewSent, .reviewError, .reviewNvidia, .reviewNvidiaTwoCabals,
            .reviewSpaceX:
            ProposeSampleRoute(scenario: self)
        }
    }
}

nonisolated struct ProposeSampleRoute: AppRoute {
    let scenario: ProposeSampleScenario

    @MainActor @ViewBuilder func destination() -> some View {
        ProposeSampleScreen(scenario: scenario)
    }
}

private struct ProposeSampleScreen: View {
    let scenario: ProposeSampleScenario
    @Environment(AppEnvironment.self) private var environment
    @State private var presentsChooser = true

    var body: some View {
        switch scenario {
        case .chooser, .chooserCashOnly, .chooserNoHoldings, .chooserNotMember:
            Color.clear.monacoCanvas().proposeChooser(
                isPresented: $presentsChooser, cabalID: GroupDetailSampleData.cabalID)
        case .buyNoResults:
            ProposeBuyStockView(cabalID: GroupDetailSampleData.cabalID, initialQuery: "zzzz")
        case .amountTyped:
            amount(seed: ProposeAmountSeed(amount: "400"))
        case .amountInsufficient:
            amount(seed: ProposeAmountSeed(amount: "9999", thesis: "Earnings next week."))
        case .amountEmptyThesis:
            amount(seed: ProposeAmountSeed(amount: "25", requestsReview: true))
        case .amountError:
            amount(seed: ProposeAmountSeed())
        default:
            ProposeReviewSample(kind: .buy, scenario: scenario)
        }
    }

    private func amount(seed: ProposeAmountSeed) -> some View {
        var screen = ProposeAmountScreen(
            service: MonacoCore.LiveProposeService(api: environment.api), cabalID: GroupDetailSampleData.cabalID,
            stock: ProposeStock(symbol: "GOOGLx", logoURL: ProposeSampleLogo.url), bestChains: environment.bestChains)
        screen.seed = seed
        return screen
    }
}

private enum ProposeSampleLogo {
    static let url = URL(string: "https://logos.example/GOOGLx.png")
}

final class ProposeSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        guard let scenario = ProposeSampleScenario.matching(arguments) else { return nil }
        SampleAPIProtocol.install(scenario.script)
        return AnyView(SampleAppFrame(auth: auth, tab: scenario.tab, routes: [scenario.route]))
    }
}

private struct ProposeSampleService: MonacoCore.ProposeService {
    enum Send { case succeeds, hangs, fails }

    let live: MonacoCore.LiveProposeService
    let send: Send

    struct Refused: Error {}

    func preview(cabalID: String, draft: ProposalDraft, network: Components.Schemas.Network?) async throws
        -> MonacoCore.ProposePreview
    {
        try await live.preview(cabalID: cabalID, draft: draft, network: network)
    }

    func propose(
        cabalID: String, draft: ProposalDraft, network: Components.Schemas.Network?, submission: IdempotentSubmission
    ) async throws -> String {
        switch send {
        case .succeeds: return "proposal-1"
        case .hangs:
            try await Task.sleep(for: .seconds(3600))
            return "proposal-1"
        case .fails: throw Refused()
        }
    }

    func withdraw(proposalID: String, submission: IdempotentSubmission) async throws {}
}

struct ProposeReviewSample: View {
    let kind: ProposeKind
    var scenario: ProposeSampleScenario = .review
    @Environment(AppEnvironment.self) private var environment
    @State private var memory = MonacoCore.ProposeReviewMemory()
    @State private var presents = true

    var body: some View {
        Color.clear
            .monacoCanvas()
            .sheet(isPresented: $presents) {
                ProposeReviewSheet(
                    service: service, cabalID: GroupDetailSampleData.cabalID, draft: draft,
                    preview: preview, trade: trade,
                    buyName: buyName, logoURL: ProposeSampleLogo.url, memory: memory,
                    sendsOnAppear: sends, onChangeCabal: scenario == .reviewNvidiaTwoCabals ? {} : nil)
            }
    }

    private var symbol: String {
        switch scenario {
        case .reviewNvidia, .reviewNvidiaTwoCabals: "NVDAx"
        case .reviewSpaceX: "SPACEX"
        default: "GOOGLx"
        }
    }

    private var buyName: String? {
        guard kind == .buy else { return nil }
        switch scenario {
        case .reviewNvidia, .reviewNvidiaTwoCabals: return "NVIDIA"
        case .reviewSpaceX: return "SpaceX"
        default: return "Alphabet"
        }
    }

    private var preview: MonacoCore.ProposePreview {
        switch scenario {
        case .reviewNvidia, .reviewNvidiaTwoCabals:
            MonacoCore.ProposePreview(
                .init(
                    quoteOutAmount: 182_000_000, potValueMicros: 20_000_000_000, spendableCashMicros: 5_100_000_000))
        case .reviewSpaceX:
            MonacoCore.ProposePreview(
                .init(
                    quoteOutAmount: 270_000_000, potValueMicros: 20_000_000_000, spendableCashMicros: 5_100_000_000))
        default: MonacoCore.ProposePreview(.proposalPreviewClean)
        }
    }

    private var live: MonacoCore.LiveProposeService { MonacoCore.LiveProposeService(api: environment.api) }

    private var service: any MonacoCore.ProposeService {
        switch scenario {
        case .reviewSubmitting: ProposeSampleService(live: live, send: .hangs)
        case .reviewSent: ProposeSampleService(live: live, send: .succeeds)
        case .reviewError: ProposeSampleService(live: live, send: .fails)
        default: live
        }
    }

    private var sends: Bool { [.reviewSubmitting, .reviewSent, .reviewError].contains(scenario) }

    private var holding: ProposeHolding {
        ProposeHolding(Components.Schemas.CabalPot.sampleInvested.holdings[0])
    }

    private var trade: MonacoCore.ProposeTrade {
        kind == .buy
            ? .buy(symbol: symbol, kind: scenario == .reviewSpaceX ? .preIpo : .stock, tokenDecimals: 8)
            : .sell(holding)
    }

    private var thesis: String {
        scenario == .reviewLongThesis
            ? String(repeating: "Earnings next week and the cloud business keeps compounding. ", count: 4)
            : "Earnings next week."
    }

    private var buyMicros: Int64 {
        switch scenario {
        case .reviewNvidia, .reviewNvidiaTwoCabals: 400_000_000
        case .reviewSpaceX: 500_000_000
        default: 25_000_000
        }
    }

    private var draft: MonacoCore.ProposalDraft {
        kind == .buy
            ? .buy(symbol: symbol, usdcMicros: buyMicros, thesis: thesis)
            : .sell(symbol: "GOOGLx", tokenAmount: holding.tokenAmount(forMicros: 25_000_000), thesis: thesis)
    }
}
#endif
