#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI

enum ParitySampleScenario: String, CaseIterable {
    case toastSettled = "toast-settled"
    case infoSheet = "info-sheet"

    static let launchArgument = "-MonacoParitySample"

    static func matching(_ arguments: [String]) -> ParitySampleScenario? {
        guard let flag = arguments.firstIndex(of: launchArgument), arguments.indices.contains(flag + 1) else {
            return nil
        }
        return ParitySampleScenario(rawValue: arguments[flag + 1])
    }

    var tab: MainTab {
        switch self {
        case .toastSettled: .home
        case .infoSheet: .stocks
        }
    }

    var routes: [any AppRoute] {
        switch self {
        case .infoSheet: [ParityInfoSheetRoute()]
        case .toastSettled: []
        }
    }

    var voteCardSetup: (@MainActor (HomeVoteCardModel) -> Void)? {
        switch self {
        case .toastSettled:
            {
                let outcome = CastOutcome(
                    choice: .yes, status: .passed, tally: ProposalTally(yes: 5, no: 1, voters: 8, needed: 5),
                    undoUntil: nil,
                    myVote: ProposalMyVote(order: 5, of: 8, yesOrder: 5, noOrder: 0, decided: true))
                $0.preview(.committed(outcome), sharesEstimate: "1.82")
            }
        case .infoSheet: nil
        }
    }

    var script: SampleAPIScript {
        var script = SampleAPIScript(role: "member")
        switch self {
        case .toastSettled:
            script = HomeSampleData.script(.loss, proposals: [.voted])
            script.proposals = script.proposals.mapValues { detail in
                var settled = detail
                settled.tally = .init(yes: 5, no: 1, voters: 8, needed: 5)
                return settled
            }
        case .infoSheet:
            script.asset = .nvda
            script.asset.priceAsOf = .now
        }
        return script
    }
}

private struct ParityToastSettledFrame: View {
    @Environment(ToastCenter.self) private var toasts
    let auth: PrivyAuthService
    let scenario: ParitySampleScenario

    var body: some View {
        SampleAppFrame(auth: auth, tab: scenario.tab, routes: scenario.routes)
            .task {
                let tally = ProposalTally(yes: 5, no: 1, voters: 6, needed: 4)
                let outcome = CastOutcome(
                    choice: .yes, status: .passed, tally: tally, undoUntil: nil, myVote: nil)
                toasts.show(success: ProposalVoteCopy.toast(outcome: outcome).message)
            }
    }
}

nonisolated struct ParityInfoSheetRoute: AppRoute {
    @MainActor func destination() -> some View { ParityInfoSheetHost() }
}

private struct ParityInfoSheetHost: View {
    @State private var shows = true

    private static var facts: MarketStatsFacts {
        MarketStatsFacts(
            listingNetworks: [.solana, .base, .robinhood], tradesContinuously: true, priceSource: .coingecko,
            priceAsOf: .now, pollInterval: .seconds(60))
    }

    var body: some View {
        AssetRoute(symbol: Components.Schemas.AssetDetail.nvda.symbol).destination()
            .sheet(isPresented: $shows) { MarketStatsSheet(facts: ParityInfoSheetHost.facts) }
    }
}

final class ParitySampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        guard let scenario = ParitySampleScenario.matching(arguments) else { return nil }
        SampleAPIProtocol.install(scenario.script)
        if scenario == .toastSettled { return AnyView(ParityToastSettledFrame(auth: auth, scenario: scenario)) }
        return AnyView(SampleAppFrame(auth: auth, tab: scenario.tab, routes: scenario.routes))
    }
}
#endif
