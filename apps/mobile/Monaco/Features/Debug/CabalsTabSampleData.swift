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
