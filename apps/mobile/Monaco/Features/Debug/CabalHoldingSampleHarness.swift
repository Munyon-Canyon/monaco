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

    static func matching(_ arguments: [String]) -> CabalHoldingFixture.Scenario? {
        guard let flag = arguments.firstIndex(of: launchArgument), arguments.indices.contains(flag + 1) else {
            return nil
        }
        return CabalHoldingFixture.Scenario(rawValue: arguments[flag + 1])
    }
}

final class CabalHoldingSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        guard let scenario = CabalHoldingSample.matching(arguments) else { return nil }
        return AnyView(SampleAppFrame(auth: auth, tab: .cabals, routes: [CabalHoldingSampleRoute(scenario: scenario)]))
    }
}
#endif
