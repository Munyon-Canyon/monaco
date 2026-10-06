#if DEBUG
import MonacoCore
import SwiftUI

/// Debug-only: renders Home from canned `AppSessionStore` data so QA can screenshot
/// each state without Privy or a backend. Launch with
/// `-MonacoHomeSample <populated|empty|loading>`.
enum HomeSampleScenario: String, CaseIterable {
    case populated
    case empty
    case loading

    static func matching(_ arguments: [String]) -> HomeSampleScenario? {
        guard let flag = arguments.firstIndex(of: "-MonacoHomeSample"),
            arguments.indices.contains(flag + 1)
        else { return nil }
        return HomeSampleScenario(rawValue: arguments[flag + 1])
    }
}

struct HomeSampleHarness: View {
    let scenario: HomeSampleScenario
    @ObservedObject var auth: PrivyAuthService
    @State private var session: AppSessionStore
    @State private var selectedTab: MainTab = .home

    init(scenario: HomeSampleScenario, auth: PrivyAuthService) {
        self.scenario = scenario
        self.auth = auth
        _session = State(initialValue: Self.makeSession(for: scenario))
    }

    var body: some View {
        NavigationStack {
            HomeView(auth: auth, selectedTab: $selectedTab)
        }
        .environment(session)
    }

    private static func makeSession(for scenario: HomeSampleScenario) -> AppSessionStore {
        let session = AppSessionStore(apiClient: MonacoAPIClient())
        session.isLoading = false

        if scenario == .loading {
            session.isLoading = true
            return session
        }

        session.profile = ProfileSampleHarness.sampleProfile(
            userID: "sample-user",
            displayName: "Logan Norman",
            photoURL: scenario == .empty ? nil : ProfileSampleHarness.samplePhotoURL()
        )

        let joined = scenario != .empty

        // Pot values for the "Your cabals" subtitles; same cabals as `ProfileSampleHarness`.
        session.home = HomeViewDTO(
            groups: joined
                ? [
                    HomeGroupBoardRowDTO(
                        groupId: "g1", name: "Weekend investors", potValueUsd: "548.20", percentReturn: "0.124",
                        dollarPnl: "+48.20", isJoined: true),
                    HomeGroupBoardRowDTO(
                        groupId: "g2", name: "Semis or bust", potValueUsd: "2310.75", percentReturn: "-0.031",
                        dollarPnl: "-73.90", isJoined: true),
                    HomeGroupBoardRowDTO(
                        groupId: "g3", name: "Index huggers", potValueUsd: "120.00", percentReturn: nil,
                        dollarPnl: "+0.00", isJoined: true),
                ] : [],
            people: []
        )

        session.dashboard = HomeDashboardDTO(
            leaderboard: HomeLeaderboardSectionDTO(
                range: "ALL",
                people: joined
                    ? [
                        HomePeopleBoardRowDTO(
                            userId: "u1", displayName: "Alfred", percentReturn: "0.124", dollarPnl: "+48.20"),
                        HomePeopleBoardRowDTO(
                            userId: "u2", displayName: "Priya Shah", percentReturn: "0.081", dollarPnl: "+22.10"),
                    ] : []
            )
        )
        return session
    }
}

final class HomeSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        guard let scenario = HomeSampleScenario.matching(arguments) else { return nil }
        return AnyView(HomeSampleHarness(scenario: scenario, auth: auth))
    }
}
#endif
