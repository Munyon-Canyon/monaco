#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI

/// Debug-only sample data for the Cabals tab. Launch with
/// `-MonacoCabalsTabSample` to open the tab without sign-in or a backend
/// (screenshots and XCUITests). Never compiled into Release builds.
enum CabalsTabSampleData {
    static let launchArgument = "-MonacoCabalsTabSample"
    static let scenarioArgument = "-MonacoCabalsTabSampleScenario"

    /// What the tab is booted into. Add a scenario rather than a second harness:
    /// the point of this file is that every Cabals screenshot comes from the
    /// same fixed cabals.
    enum Scenario: String {
        /// Signed in, cabals loaded. The default.
        case normal
        /// The cabals list came back empty-handed and nothing is in flight: the
        /// read failed. The strip offers a retry.
        case cabalsUnavailable
        /// The shell's first load is still running, so the cabals list is not
        /// late — it has not arrived yet. The strip shows placeholder cards.
        case cabalsLoading
    }

    static func matches(_ arguments: [String]) -> Bool {
        arguments.contains(launchArgument)
    }

    static var scenario: Scenario {
        let arguments = ProcessInfo.processInfo.arguments
        guard let flag = arguments.firstIndex(of: scenarioArgument),
            arguments.indices.contains(flag + 1),
            let scenario = Scenario(rawValue: arguments[flag + 1])
        else { return .normal }
        return scenario
    }

    struct Cabal {
        let id: String
        let name: String
        let members: Int
        let pot: String
        let pnl: String
        let percent: String?
        let joined: Bool
        let mode: GroupJoinMode
    }

    static let cabals: [Cabal] = [
        Cabal(
            id: "5b1f0c9e-0001-4c55-9a51-000000000001", name: "Weekend investors", members: 4, pot: "548.20",
            pnl: "+48.20", percent: "0.0964", joined: true, mode: .open),
        Cabal(
            id: "5b1f0c9e-0002-4c55-9a51-000000000002", name: "Rent money", members: 3, pot: "212.40", pnl: "-7.60",
            percent: "-0.0345", joined: true, mode: .request),
        Cabal(
            id: "5b1f0c9e-0003-4c55-9a51-000000000003", name: "Apple heads", members: 2, pot: "91.35", pnl: "+1.35",
            percent: "0.015", joined: true, mode: .open),
        Cabal(
            id: "5b1f0c9e-0004-4c55-9a51-000000000004", name: "Dorm 4B fund", members: 9, pot: "1320.00",
            pnl: "+320.00", percent: "0.32", joined: false, mode: .open),
        Cabal(
            id: "5b1f0c9e-0005-4c55-9a51-000000000005", name: "Tesla or bust", members: 5, pot: "760.10",
            pnl: "+110.10", percent: "0.1694", joined: false, mode: .request),
        Cabal(
            id: "5b1f0c9e-0006-4c55-9a51-000000000006", name: "Weekend warriors", members: 3, pot: "64.00",
            pnl: "-6.00", percent: "-0.0857", joined: false, mode: .open),
    ]

    static var home: HomeViewDTO {
        HomeViewDTO(
            groups: cabals.filter(\.joined).map {
                HomeGroupBoardRowDTO(
                    groupId: $0.id, name: $0.name, potValueUsd: $0.pot, percentReturn: $0.percent, dollarPnl: $0.pnl,
                    isJoined: true)
            },
            people: []
        )
    }

    static let createdCabalID = "5b1f0c9e-0007-4c55-9a51-000000000007"

    /// A stubbed create that always succeeds.
    @MainActor
    struct Actions: CabalsActionSource {
        func createCabal(_ input: CreateCabalInput, submission: IdempotentSubmission) async throws
            -> Components.Schemas.Cabal
        {
            try await Task.sleep(for: .milliseconds(120))
            var created = Components.Schemas.Cabal.sample(role: "creator")
            created.id = createdCabalID
            created.name = input.name
            return created
        }
    }
}

/// Root view for `-MonacoCabalsTabSample`: the Cabals tab on sample data.
struct CabalsTabSampleHarness: View {
    @ObservedObject var auth: PrivyAuthService
    @State private var session: AppSessionStore = {
        let session = AppSessionStore(apiClient: MonacoAPIClient())
        // `home` stays nil in both unhappy scenarios; what separates them is
        // whether the shell still has a load running, which is what tells
        // "not here yet" from "did not arrive".
        if CabalsTabSampleData.scenario == .normal {
            session.home = CabalsTabSampleData.home
        }
        session.isLoading = CabalsTabSampleData.scenario == .cabalsLoading
        return session
    }()

    var body: some View {
        TabView {
            NavigationStack {
                CabalsTabView(auth: auth, actions: CabalsTabSampleData.Actions())
            }
            .tabItem {
                Label("Cabals", systemImage: "person.3")
                    .accessibilityIdentifier("tab-cabals")
            }
        }
        .tint(MonacoTheme.ink)
        .environment(session)
    }
}

final class CabalsTabSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        guard CabalsTabSampleData.matches(arguments) else { return nil }
        return AnyView(CabalsTabSampleHarness(auth: auth))
    }
}
#endif
