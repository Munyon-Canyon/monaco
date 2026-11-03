#if DEBUG
import MonacoCore
import SwiftUI

nonisolated struct CabalHoldingSampleRoute: AppRoute {
    let scenario: CabalHoldingFixture.Scenario

    @MainActor func destination() -> some View {
        CabalHoldingSampleHost(scenario: scenario)
    }
}

private struct CabalHoldingSampleHost: View {
    @State private var model: CabalHoldingModel

    init(scenario: CabalHoldingFixture.Scenario) {
        _model = State(initialValue: CabalHoldingFixture.model(scenario, now: Date()))
    }

    var body: some View {
        CabalHoldingScreen(model: model) { _ in }
    }
}

enum CabalHoldingSample {
    static let launchArgument = "-MonacoCabalHoldingSample"

}

final class CabalHoldingSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        return scenarioRoot(
            CabalHoldingFixture.Scenario.self, flag: CabalHoldingSample.launchArgument, arguments: arguments
        ) { scenario in
            return AnyView(
                SampleAppFrame(auth: auth, tab: .cabals, routes: [CabalHoldingSampleRoute(scenario: scenario)]))
        }
    }
}
#endif
