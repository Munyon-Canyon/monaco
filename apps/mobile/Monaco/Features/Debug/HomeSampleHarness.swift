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
        let session = AppSessionStore()
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

        session.hasLoaded = true
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
