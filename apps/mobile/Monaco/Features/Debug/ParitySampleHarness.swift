#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI

enum ParitySampleScenario: String, CaseIterable {
    case addMoneyError = "add-money-error"
    case cabalInfoError = "cabal-info-error"
    case feedEveryone = "feed-everyone"
    case feedMine = "feed-mine"
    case membersError = "members-error"
    case publicCabalError = "public-cabal-error"
    case toastSettled = "toast-settled"
    case infoSheet = "info-sheet"
    case votesClosed = "votes-closed"
    case votesExpired = "votes-expired"
    case votesRejected = "votes-rejected"

    static let launchArgument = "-MonacoParitySample"

    static func matching(_ arguments: [String]) -> ParitySampleScenario? {
        guard let flag = arguments.firstIndex(of: launchArgument), arguments.indices.contains(flag + 1) else {
            return nil
        }
        return ParitySampleScenario(rawValue: arguments[flag + 1])
    }

    var feedScope: FeedScope? {
        switch self {
        case .feedEveryone: .everyone
        case .feedMine: .mine
        default: nil
        }
    }

    var tab: MainTab {
        switch self {
        case .addMoneyError, .toastSettled: .home
        case .infoSheet: .stocks
        default: .cabals
        }
    }

    var routes: [any AppRoute] {
        let id = GroupDetailSampleData.cabalID
        switch self {
        case .addMoneyError: return []
        case .cabalInfoError: return [CabalRoute(id: id, tab: .rules)]
        case .membersError: return [MembersRoute(cabalID: id)]
        case .publicCabalError: return [CabalSampleRoute(kind: .publicMember)]
        case .infoSheet: return [ParityInfoSheetRoute()]
        case .votesClosed, .votesExpired, .votesRejected: return [ProposalSampleRoute(votes: .open)]
        case .toastSettled, .feedEveryone, .feedMine: return []
        }
    }

    var script: SampleAPIScript {
        var script = SampleAPIScript(role: "member")
        switch self {
        case .addMoneyError: script.mode = .balanceFails
        case .cabalInfoError, .membersError, .publicCabalError: script.mode = .offline
        case .votesClosed: script.proposal = .sample(status: .executed)
        case .votesExpired: script.proposal = .sample(status: .expired)
        case .votesRejected: script.proposal = .sample(status: .failed)
        case .toastSettled:
            script = HomeSampleScenario.voted.script
            script.proposals = script.proposals.mapValues { detail in
                var settled = detail
                settled.tally = .init(yes: 5, no: 1, voters: 8, needed: 5)
                return settled
            }
        case .infoSheet:
            script.asset = .nvda
            script.asset.priceAsOf = .now
        case .feedEveryone, .feedMine: break
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

private struct ParityFeedScreen: View {
    let scope: FeedScope

    @State private var path: [AnyAppRoute] = []

    var body: some View {
        NavigationStack(path: $path) {
            FeedView(makeModel: { _ in
                let model = FeedModel.preview(.items, clock: ContinuousClock())
                Task { await model.select(scope) }
                return model
            })
            .navigationDestination(for: AnyAppRoute.self) { $0.destination() }
        }
    }
}

final class ParitySampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        guard let scenario = ParitySampleScenario.matching(arguments) else { return nil }
        SampleAPIProtocol.install(scenario.script)
        if let scope = scenario.feedScope { return AnyView(ParityFeedScreen(scope: scope)) }
        if scenario == .toastSettled { return AnyView(ParityToastSettledFrame(auth: auth, scenario: scenario)) }
        return AnyView(
            SampleAppFrame(
                auth: auth, tab: scenario.tab, routes: scenario.routes, presentsDeposit: scenario == .addMoneyError))
    }
}
#endif
