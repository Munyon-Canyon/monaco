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
    case nudgeX
    case nudgeXEmpty
    case prePrompt

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
        case .nudgeX: SampleAPIScript()
        case .nudgeXEmpty: SampleAPIScript(mode: .empty)
        case .prePrompt: SampleAPIScript()
        }
    }
}

private struct SampleNotificationAuthorizing: NotificationAuthorizing {
    func status() async -> PushAuthorization { .notDetermined }
    func request() async -> Bool { false }
}

private enum SamplePushPrePrompt {
    @MainActor static func make() -> PushPrePrompt {
        PushPrePrompt(
            authorization: SampleNotificationAuthorizing(),
            defaults: UserDefaults(suiteName: "sample-push-pre-prompt") ?? .standard,
            register: {})
    }
}

struct HomeSampleHarness: View {
    let scenario: HomeSampleScenario
    @ObservedObject var auth: PrivyAuthService

    var body: some View {
        SampleAppFrame(auth: auth, tab: .home, session: session, sheet: sheet)
    }

    private var sheet: (() -> AnyView)? {
        guard scenario == .prePrompt else { return nil }
        return { AnyView(PushPrePromptSheet(prompt: SamplePushPrePrompt.make())) }
    }

    private func session(_ store: AppSessionStore) {
        if scenario == .loading {
            SampleAppFrame.loading(store)
        } else {
            SampleAppFrame.signedIn(store)
            let connectX = ProcessInfo.processInfo.arguments.contains("-MonacoFeatureConnectX")
            if connectX && (scenario == .nudgeX || scenario == .nudgeXEmpty) {
                store.profile?.authState = .awaitingSocials
            }
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
