#if DEBUG
import Foundation
import MonacoAPI
import MonacoCore
import SwiftUI

enum CatchUpSampleScenario: String {
    case six, caughtUp, firstUse, loading, error

    static let launchArgument = "-MonacoCatchUpSample"
    static let legacyArgument = "-activityFeedHarness"

    static func matching(_ arguments: [String]) -> CatchUpSampleScenario? {
        if let flag = arguments.firstIndex(of: launchArgument), arguments.indices.contains(flag + 1) {
            return CatchUpSampleScenario(rawValue: arguments[flag + 1])
        }
        guard let flag = arguments.firstIndex(of: legacyArgument) else { return nil }
        let name = arguments.indices.contains(flag + 1) ? arguments[flag + 1] : "unread"
        switch name {
        case "loading": return .loading
        case "error": return .error
        default: return .six
        }
    }

    var state: LoadState<CatchUpScreen> {
        switch self {
        case .six: .loaded(CatchUpScreen(.six))
        case .caughtUp: .loaded(CatchUpScreen(.caughtUp))
        case .firstUse: .loaded(CatchUpScreen(.firstUse))
        case .loading: .loading
        case .error: .failed(.transport(URLError(.notConnectedToInternet)))
        }
    }
}

nonisolated struct CatchUpSampleRoute: AppRoute {
    enum Outcome: Hashable {
        case scenario(CatchUpSampleScenario)
        case unauthorized
    }

    let outcome: Outcome

    @MainActor func destination() -> some View {
        let now = Components.Schemas.CatchUp.sampleNow
        let state: LoadState<CatchUpScreen>
        switch outcome {
        case .scenario(let scenario): state = scenario.state
        case .unauthorized: state = .failed(.signedOut)
        }
        return CatchUpView(model: .preview(state: state, now: now), now: { now })
    }
}

final class CatchUpSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        guard let scenario = CatchUpSampleScenario.matching(arguments) else { return nil }
        return AnyView(
            SampleAppFrame(auth: auth, tab: .home, routes: [CatchUpSampleRoute(outcome: .scenario(scenario))]))
    }
}

final class CatchUpFlowHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        guard let scenario = Flow34Scenario.matching(arguments) else { return nil }
        switch scenario {
        case .unauthorized:
            return AnyView(
                SampleAppFrame(auth: auth, tab: .home, routes: [CatchUpSampleRoute(outcome: .unauthorized)]))
        }
    }
}
#endif
