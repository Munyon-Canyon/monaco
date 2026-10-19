import MonacoCore
import SwiftUI

// The login views take `PrivyAuthService` itself. They used to be generic over a protocol so
// the Debug harness could hand them a canned sign-in; the async witness call through that
// generic crashed with a bus error in `swift_retain` on the first "Send code" (a clean build
// did not help). The harness now subclasses the service instead, which is plain class
// dispatch and works.

/// The two ways in. Which of them are on comes from the build (`Config.privy`).
enum LoginMethod: String, CaseIterable, Identifiable {
    case sms = "Text message"
    case email = "Email"

    var id: String { rawValue }

    /// The methods turned on, texting first.
    static func available(sms: Bool, email: Bool) -> [LoginMethod] {
        allCases.filter { $0 == .sms ? sms : email }
    }

    static var configured: [LoginMethod] {
        available(sms: Config.privy.smsLoginEnabled, email: Config.privy.emailLoginEnabled)
    }
}

/// Sign-in, as two layouts over one form. The address step runs from the wordmark down: the intro,
/// the method, the field and the button, then the cover card. The code step swaps all of that for
/// its own bar and boxes. The form keeps its button above the keyboard while a field is in use, so
/// the phone pad (which has no return key) never hides the only way forward.
struct LoginView: View {
    @ObservedObject var auth: PrivyAuthService

    private let methods: [LoginMethod]
    /// Debug harness only: the code and the address the form opens with, to shoot them typed.
    private let initialCode: String
    private let initialAddress: String

    @State private var selectedMethod: LoginMethod
    @Environment(ToastCenter.self) private var toasts
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    init(
        auth: PrivyAuthService,
        methods: [LoginMethod] = LoginMethod.configured,
        initialMethod: LoginMethod? = nil,
        initialCode: String = "",
        initialAddress: String = ""
    ) {
        self.auth = auth
        self.methods = methods
        self.initialCode = initialCode
        self.initialAddress = initialAddress
        _selectedMethod = State(initialValue: initialMethod ?? methods.first ?? .sms)
    }

    private var onAddressStep: Bool { !auth.flow.isOnCodeStep }

    private var showsAddressChrome: Bool { onAddressStep && !auth.flow.isOffline }

    var body: some View {
        ScrollViewReader { proxy in
            ScrollView {
                VStack(alignment: .leading, spacing: 0) {
                    if onAddressStep {
                        LaunchScreenView()
                            .padding(.bottom, MonacoTheme.Space.l)
                            .authStepTransition(reduceMotion: reduceMotion)
                        if let reason = auth.lastSignOutReason {
                            signOutNotice(reason)
                        }
                    }
                    if showsAddressChrome, methods.count > 1 {
                        methodPicker
                    }
                    form(scroll: proxy)
                    if showsAddressChrome {
                        AuthCoverCard()
                            .authStepTransition(reduceMotion: reduceMotion)
                        #if DEBUG
                        DevSignInButton()
                        #endif
                    }
                }
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .padding(.bottom, MonacoTheme.Space.l)
                .animation(reduceMotion ? nil : .easeOut(duration: 0.2), value: auth.flow.isOnCodeStep)
                .animation(reduceMotion ? nil : .easeOut(duration: 0.2), value: auth.flow.isOffline)
            }
            .scrollBounceBehavior(.basedOnSize)
            .scrollDismissesKeyboard(.interactively)
        }
        .authScreenBackground()
        .analyticsScreen("login")
        .tint(MonacoTheme.accent)
        .foregroundStyle(MonacoTheme.primaryText)
        .onChange(of: selectedMethod) { _, _ in
            auth.resetLoginFlow()
        }
        .onChange(of: auth.flow.toastMessage) { _, message in
            guard let message else { return }
            toasts.current = MonacoToast(message: message)
        }
        .onChange(of: auth.flow.isOffline) { _, offline in
            guard offline else { return }
            toasts.current = MonacoToast(message: OTPCode.offlineMessage)
        }
    }

    private func signOutNotice(_ reason: String) -> some View {
        MonacoNotice(.info, message: reason)
            .accessibilityIdentifier("signOutReasonNotice")
            .padding(.bottom, MonacoTheme.Space.sm)
    }

    private var methodPicker: some View {
        MonacoSegmented(methods, selection: $selectedMethod) { $0.rawValue }
            .accessibilityElement(children: .contain)
            .accessibilityLabel("Sign-in method")
            // Switching method mid-send would leave the in-flight request to report
            // success against the other form: an email screen showing a code step
            // whose code was texted to a phone number.
            .disabled(auth.flow.isBusy)
            .padding(.bottom, MonacoTheme.Space.m)
            .authStepTransition(reduceMotion: reduceMotion)
    }

    @ViewBuilder
    private func form(scroll: ScrollViewProxy) -> some View {
        switch effectiveMethod {
        case .sms:
            SMSLoginView(auth: auth, scroll: scroll, initialCode: initialCode, initialAddress: initialAddress)
        case .email:
            EmailLoginView(auth: auth, scroll: scroll, initialCode: initialCode, initialAddress: initialAddress)
        }
    }

    private var effectiveMethod: LoginMethod {
        methods.contains(selectedMethod) ? selectedMethod : (methods.first ?? .sms)
    }
}

#Preview {
    LoginView(auth: PrivyAuthService())
}
