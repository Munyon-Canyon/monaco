#if DEBUG
import Foundation
import MonacoAPI
import MonacoCore
import SwiftUI

enum CatchUpSampleScenario: String {
    case six, loading, error, caughtUp, firstUse, deck, deckScrolled, deckPostReply

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

    var deckStart: CatchUpDeckStart? {
        switch self {
        case .deck: CatchUpDeckStart(key: "comment:00000000-0000-7000-8000-00000000d001")
        case .deckScrolled: CatchUpDeckStart(key: "comment:00000000-0000-7000-8000-00000000d001", scrolledToEnd: true)
        case .deckPostReply: CatchUpDeckStart(key: "comment:00000000-0000-7000-8000-00000000d003")
        case .six, .loading, .error, .caughtUp, .firstUse: nil
        }
    }

    var state: LoadState<CatchUpScreen> {
        switch self {
        case .six: .loaded(CatchUpScreen(.six))
        case .caughtUp: .loaded(CatchUpScreen(.caughtUp))
        case .firstUse: .loaded(CatchUpScreen(.firstUse))
        case .deck, .deckScrolled: .loaded(CatchUpScreen(.deck))
        case .deckPostReply: .loaded(CatchUpScreen(.deckPostReply))
        case .loading: .loading
        case .error: .failed(.transport(URLError(.notConnectedToInternet)))
        }
    }
}

nonisolated struct CatchUpSampleRoute: AppRoute {
    let scenario: CatchUpSampleScenario

    @MainActor func destination() -> some View {
        let now = Components.Schemas.CatchUp.sampleNow
        return CatchUpView(
            model: .preview(state: scenario.state, now: now), now: { now }, opensDeckAt: scenario.deckStart)
    }
}

final class CatchUpSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        guard let scenario = CatchUpSampleScenario.matching(arguments) else { return nil }
        return AnyView(SampleAppFrame(auth: auth, tab: .home, routes: [CatchUpSampleRoute(scenario: scenario)]))
    }
}

final class CatchUpFlowHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        guard Flow34Scenario.matching(arguments) != nil else { return nil }
        return AnyView(SampleAppFrame(auth: auth, tab: .home, routes: [CatchUpSampleRoute(scenario: .error)]))
    }
}
#endif
