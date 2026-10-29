#if DEBUG
import MonacoAPI
import MonacoCore
import MonacoRanking
import SwiftUI

enum LeaderboardSampleScenario: String, CaseIterable {
    case peopleMonth
    case leadChange
    case empty
    case loading
    case error

    static let launchArgument = "-MonacoLeaderboardSample"

    static func matching(_ arguments: [String]) -> LeaderboardSampleScenario? {
        guard let flag = arguments.firstIndex(of: launchArgument), arguments.indices.contains(flag + 1) else {
            return nil
        }
        return LeaderboardSampleScenario(rawValue: arguments[flag + 1])
    }

    var state: LoadState<CallBoardScreen> {
        switch self {
        case .peopleMonth: .loaded(CallBoardScreen(.peopleMonth))
        case .leadChange: .loaded(CallBoardScreen(.peopleMonthLeadChange))
        case .empty: .loaded(CallBoardScreen(.empty))
        case .loading: .loading
        case .error: .failed(.transport(URLError(.notConnectedToInternet)))
        }
    }
}

nonisolated struct LeaderboardSampleRoute: AppRoute {
    let scenario: LeaderboardSampleScenario

    @MainActor func destination() -> some View {
        LeaderboardView(tab: .people, period: .month, preview: .preview(state: scenario.state))
    }
}

final class LeaderboardSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        guard let scenario = LeaderboardSampleScenario.matching(arguments) else { return nil }
        SampleAPIProtocol.install(SampleAPIScript())
        return AnyView(SampleAppFrame(auth: auth, tab: .cabals, routes: [LeaderboardSampleRoute(scenario: scenario)]))
    }
}
#endif
