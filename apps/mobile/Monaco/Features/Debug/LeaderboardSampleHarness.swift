#if DEBUG
import MonacoAPI
import MonacoCore
import MonacoFlows
import MonacoRanking
import SwiftUI

enum LeaderboardSampleScenario: String, CaseIterable {
    case peopleMonth
    case peopleLong
    case leadChange
    case cabalsMonth
    case cabalsUnranked
    case peopleWeekUnranked
    case friendsMonth
    case empty
    case loading
    case error

    static let launchArgument = "-MonacoLeaderboardSample"

    var tab: CallBoardTab {
        switch self {
        case .cabalsMonth, .cabalsUnranked: .cabals
        case .friendsMonth: .friends
        default: .people
        }
    }

    var period: CallPeriod { self == .peopleWeekUnranked ? .week : .month }

    var state: LoadState<CallBoardScreen> {
        switch self {
        case .peopleMonth: .loaded(CallBoardScreen(.peopleMonth))
        case .peopleLong: .loaded(CallBoardScreen(.peopleMonthLong))
        case .leadChange: .loaded(CallBoardScreen(.peopleMonthLeadChange))
        case .cabalsMonth: .loaded(CallBoardScreen(.cabalsMonth))
        case .cabalsUnranked: .loaded(CallBoardScreen(.cabalsUnranked))
        case .peopleWeekUnranked: .loaded(CallBoardScreen(.peopleWeekUnranked))
        case .friendsMonth: .loaded(CallBoardScreen(.friendsMonth))
        case .empty: .loaded(CallBoardScreen(.empty))
        case .loading: .loading
        case .error: .failed(.transport(URLError(.notConnectedToInternet)))
        }
    }
}

nonisolated struct LeaderboardSampleRoute: AppRoute {
    let scenario: LeaderboardSampleScenario

    @MainActor func destination() -> some View {
        LeaderboardView(
            tab: scenario.tab, period: scenario.period,
            preview: .preview(state: scenario.state, tab: scenario.tab, period: scenario.period))
    }
}

final class LeaderboardSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        return scenarioRoot(
            LeaderboardSampleScenario.self, flag: LeaderboardSampleScenario.launchArgument, arguments: arguments
        ) { scenario in
            SampleAPIProtocol.install(SampleAPIScript())
            return AnyView(
                SampleAppFrame(auth: auth, tab: .home, routes: [LeaderboardSampleRoute(scenario: scenario)]))
        }
    }
}

final class LeaderboardFlowHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        guard Flow19aScenario.matching(arguments) != nil || Flow19bScenario.matching(arguments) != nil else {
            return nil
        }
        return AnyView(SampleAppFrame(auth: auth, tab: .cabals, routes: [LeaderboardSampleRoute(scenario: .error)]))
    }
}
#endif
