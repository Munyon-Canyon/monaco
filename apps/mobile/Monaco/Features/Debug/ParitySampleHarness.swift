#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI

enum ParitySampleScenario: String, CaseIterable {
    case addMoneyError = "add-money-error"
    case cabalInfoError = "cabal-info-error"
    case feedPriceMoves = "feed-price-moves"
    case feedProposals = "feed-proposals"
    case feedTrades = "feed-trades"
    case membersError = "members-error"
    case publicCabalError = "public-cabal-error"
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

    var feedGroup: FeedKindGroup? {
        switch self {
        case .feedPriceMoves: .priceMoves
        case .feedProposals: .proposals
        case .feedTrades: .completedTrades
        default: nil
        }
    }

    var tab: MainTab {
        switch self {
        case .addMoneyError: .home
        default: .cabals
        }
    }

    var routes: [any AppRoute] {
        let id = GroupDetailSampleData.cabalID
        switch self {
        case .addMoneyError: return []
        case .cabalInfoError: return [CabalInfoRoute(cabalID: id)]
        case .membersError: return [MembersRoute(cabalID: id, tab: .members)]
        case .publicCabalError: return [CabalSampleRoute(kind: .publicMember)]
        case .votesClosed, .votesExpired, .votesRejected: return [ProposalSampleRoute(votes: .open)]
        case .feedPriceMoves, .feedProposals, .feedTrades: return []
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
        case .feedPriceMoves, .feedProposals, .feedTrades: break
        }
        return script
    }
}

private struct ParityFeedScreen: View {
    let group: FeedKindGroup

    @State private var path: [AnyAppRoute] = []

    var body: some View {
        NavigationStack(path: $path) {
            FeedView(makeModel: { _ in
                let model = FeedModel.preview(.items, clock: ContinuousClock())
                Task { await model.toggle(group) }
                return model
            })
            .navigationDestination(for: AnyAppRoute.self) { $0.destination() }
        }
        .environment(
            \.feedOpensSoon,
            FeedOpensSoonData(hasUnreadActivity: true)
        )
    }
}

final class ParitySampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        guard let scenario = ParitySampleScenario.matching(arguments) else { return nil }
        SampleAPIProtocol.install(scenario.script)
        if let group = scenario.feedGroup { return AnyView(ParityFeedScreen(group: group)) }
        return AnyView(
            SampleAppFrame(
                auth: auth, tab: scenario.tab, routes: scenario.routes, presentsDeposit: scenario == .addMoneyError))
    }
}
#endif
