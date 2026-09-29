#if DEBUG
import Combine
import MonacoCore
import SwiftUI

/// Debug-only: the way in (sign-in, the restore, the session gate) against canned state, so QA
/// can screenshot every step of it without Privy or a backend. Launch with
/// `-MonacoWelcomeSample <scenario>`; the bare flag means `code`.
///
/// The sign-in scenarios drive the real `LoginView` through `SampleSignIn`, the service with the
/// network taken out, and `SampleTextMessage`: sending a code succeeds after a beat and every code
/// is turned down, so the screen stays put to be looked at. The gate scenarios draw the gate's own restoring, loading and failure views.
enum WelcomeSampleScenario: String, CaseIterable {
    /// Back on login after the session ended, with the reason over the form.
    case signedOut
    /// A code went to a phone number; the code field is empty.
    case code
    /// The six digits typed in were turned down.
    case codeRejected
    /// Launch, restoring a saved sign-in: the launch mark, held.
    case restoring
    /// The saved sign-in couldn't be checked (offline).
    case restoreFailed
    /// Signed in, and the backend session is opening: Home's shape.
    case gateLoading
    /// The backend session wouldn't open.
    case gateFailed
    /// `EmptyState` where the app puts it: between a section's rules, and under a bare header.
    case emptyStates

    static let launchArgument = "-MonacoWelcomeSample"

    static var requested: WelcomeSampleScenario? {
        let arguments = ProcessInfo.processInfo.arguments
        guard let flag = arguments.firstIndex(of: launchArgument) else { return nil }
        guard arguments.indices.contains(flag + 1),
              let scenario = WelcomeSampleScenario(rawValue: arguments[flag + 1])
        else { return .code }
        return scenario
    }
}

struct WelcomeSampleHarness: View {
    let scenario: WelcomeSampleScenario
    @StateObject private var signIn: SampleSignIn

    init(scenario: WelcomeSampleScenario) {
        self.scenario = scenario
        _signIn = StateObject(wrappedValue: SampleSignIn(scenario: scenario))
    }

    var body: some View {
        switch scenario {
        case .signedOut:
            LoginView(auth: signIn)
        case .code:
            LoginView(auth: signIn, textMessage: SampleTextMessage(flow: .onCodeStep(sentTo: "+15551234567")))
        case .codeRejected:
            LoginView(
                auth: signIn,
                textMessage: SampleTextMessage(flow: .rejected(sentTo: "+15551234567")),
                initialCode: "465354"
            )
        case .restoring:
            SessionRestoringView()
        case .restoreFailed:
            SessionFailureView(
                title: SessionGateCopy.restoreFailedTitle,
                message: LoginFailureCopy.restoreOffline,
                onRetry: {},
                onSignOut: {}
            )
        case .gateLoading:
            SessionGateSkeleton()
        case .gateFailed:
            SessionFailureView(
                title: SessionGateCopy.openFailedTitle,
                message: "Can't reach Monaco. Check your connection and try again.",
                detail: "URLError notConnectedToInternet\nlocalhost:8080",
                onRetry: {},
                onSignOut: {}
            )
        case .emptyStates:
            EmptyStateSamples()
        }
    }
}

/// A canned sign-in: the real service with the network taken out, so a sample never leaves the
/// login screen.
final class SampleSignIn: PrivyAuthService {
    init(scenario: WelcomeSampleScenario) {
        super.init(settings: Config.privy)
        phase = .idle
        if scenario == .signedOut {
            lastSignOutReason = LoginFailureCopy.sessionExpired
        }
    }
}

/// A canned text-message form. Sending a code always goes through after a beat; checking one
/// always fails as a wrong code.
final class SampleTextMessage: OTPSession {
    init(flow: OTPFlow) {
        super.init()
        self.flow = flow
    }

    override func deliverCode(to destination: String) async throws {
        try? await Task.sleep(for: .milliseconds(600))
    }

    override func redeemCode(_ code: String, sentTo destination: String) async throws {
        try? await Task.sleep(for: .milliseconds(600))
        throw LoginFailure.codeRejected
    }
}

private extension OTPFlow {
    static func onCodeStep(sentTo destination: String) -> OTPFlow {
        var flow = OTPFlow()
        _ = flow.beginSend()
        flow.sendSucceeded(destination: destination)
        return flow
    }

    static func rejected(sentTo destination: String) -> OTPFlow {
        var flow = onCodeStep(sentTo: destination)
        _ = flow.beginVerify()
        flow.verifyFailed(message: OTPCode.rejectedMessage)
        return flow
    }
}

/// `EmptyState` in the places the app uses it, to check it against the rules around it.
private struct EmptyStateSamples: View {
    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.xl) {
                    section("Holdings") {
                        MonacoGroupedList {
                            EmptyState(
                                title: "Nothing bought yet",
                                message: "Add money, then propose the first buy.",
                                actionTitle: "Add money"
                            ) {}
                        }
                    }
                    section("Top cabals") {
                        MonacoGroupedList {
                            EmptyState(
                                title: "No cabal has put money in yet",
                                message: "The first one to fund takes the top spot."
                            )
                        }
                    }
                    section("Your cabals") {
                        EmptyState(
                            title: "No cabals yet",
                            message: "Start one with friends or join an open one.",
                            actionTitle: "Browse cabals"
                        ) {}
                    }
                }
                .padding(.vertical, MonacoTheme.Space.m)
            }
            .monacoCanvas()
            .navigationTitle("Empty states")
        }
    }

    private func section<Content: View>(_ title: String, @ViewBuilder content: () -> Content) -> some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            MonacoSectionHeader(title)
                .padding(.horizontal, MonacoTheme.Space.m)
            content()
        }
    }
}
#endif
