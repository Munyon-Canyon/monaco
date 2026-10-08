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

extension HomeSampleScenario {
    var script: SampleAPIScript {
        switch self {
        case .populated: SampleAPIScript()
        case .empty: SampleAPIScript(mode: .empty)
        case .loading: SampleAPIScript(mode: .hang)
        }
    }
}

struct HomeSampleHarness: View {
    let scenario: HomeSampleScenario
    @ObservedObject var auth: PrivyAuthService

    var body: some View {
        SampleAppFrame(auth: auth, tab: .home, session: session)
    }

    private func session(_ store: AppSessionStore) {
        if scenario == .loading {
            SampleAppFrame.loading(store)
        } else {
            SampleAppFrame.signedIn(store)
        }
    }
}

final class HomeSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        guard let scenario = HomeSampleScenario.matching(arguments) else { return nil }
        SampleAPIProtocol.install(scenario.script)
        return AnyView(HomeSampleHarness(scenario: scenario, auth: auth))
    }
}
#endif
