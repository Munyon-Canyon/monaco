import AuthenticationServices
import MonacoCore
import SwiftUI

// The login views take `PrivyAuthService` itself. They used to be generic over a protocol so
// the Debug harness could hand them a canned sign-in; the async witness call through that
// generic crashed with a bus error in `swift_retain` on the first "Send code" (a clean build
// did not help). The harness now subclasses the service instead, which is plain class
// dispatch and works.

enum LoginCopy {
    static let continueWithGoogle = "Continue with Google"
    #if DEBUG
    static let devTextMessage = "Dev: text message"
    #endif
}

/// Sign-in, as one composition from the top of the screen down: the brand, then the ways in.
struct LoginView: View {
    @ObservedObject var auth: PrivyAuthService

    @State private var toast: MonacoToast?

    #if DEBUG
    /// The dev text-message form, once "Dev: text message" is tapped.
    @State private var textMessage: OTPSession?
    /// Debug harness only: the code the form opens with, to shoot a typed code.
    private let initialCode: String

    init(auth: PrivyAuthService, textMessage: OTPSession? = nil, initialCode: String = "") {
        self.auth = auth
        _textMessage = State(initialValue: textMessage)
        self.initialCode = initialCode
    }
    #endif

    var body: some View {
        ScrollViewReader { proxy in
            ScrollView {
                VStack(alignment: .leading, spacing: 0) {
                    LaunchScreenView()
                    waysIn(scroll: proxy)
                        .padding(.top, 40)
                }
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .padding(.top, 72)
                .padding(.bottom, MonacoTheme.Space.l)
            }
            .scrollBounceBehavior(.basedOnSize)
            .scrollDismissesKeyboard(.interactively)
        }
        .authScreenBackground()
        .tint(MonacoTheme.accent)
        .foregroundStyle(MonacoTheme.primaryText)
        .monacoToast($toast)
        .onChange(of: auth.phase) { _, phase in
            guard let message = phase.toastMessage else { return }
            toast = MonacoToast(message: message)
        }
    }

    private func waysIn(scroll: ScrollViewProxy) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            // Why the member is here without having signed out: a quiet line over the buttons,
            // not a banner, gone with the next sign-in attempt.
            if let reason = auth.lastSignOutReason {
                Text(reason)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.loss)
                    .fixedSize(horizontal: false, vertical: true)
                    .accessibilityIdentifier("signOutReasonNotice")
                    .padding(.bottom, MonacoTheme.Space.sm)
            }

            #if DEBUG
            if let textMessage {
                SMSLoginView(session: textMessage, scroll: scroll, initialCode: initialCode)
            } else {
                providerButtons
                if Config.privy.smsLoginEnabled {
                    devTextMessageLink
                        .padding(.top, MonacoTheme.Space.s)
                }
            }
            #else
            providerButtons
            #endif
        }
        .monacoFullWidthButtons()
    }

    private var providerButtons: some View {
        VStack(spacing: MonacoTheme.Space.s) {
            // Privy runs the Apple request itself, so the system button is only drawn: the tap
            // goes to the Button around it.
            Button {
                Task { await auth.loginWithApple() }
            } label: {
                SignInWithAppleButton(.signIn, onRequest: { _ in }, onCompletion: { _ in })
                    .signInWithAppleButtonStyle(.black)
                    .frame(height: MonacoButtonMetrics.minimumHeight)
                    .clipShape(Capsule())
                    .allowsHitTesting(false)
                    .contentShape(Capsule())
            }
            .buttonStyle(.plain)
            .accessibilityLabel("Sign in with Apple")
            .accessibilityIdentifier("signInWithAppleButton")

            Button {
                Task { await auth.loginWithGoogle() }
            } label: {
                Text(LoginCopy.continueWithGoogle)
            }
            .buttonStyle(.monacoSecondary)
            .accessibilityIdentifier("continueWithGoogleButton")
        }
        .disabled(auth.phase.isBusy)
    }

    #if DEBUG
    private var devTextMessageLink: some View {
        Button {
            textMessage = PrivySMSLogin(auth: auth)
        } label: {
            Text(LoginCopy.devTextMessage)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
                .frame(minHeight: 44)
                .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .disabled(auth.phase.isBusy)
        .accessibilityIdentifier("devTextMessageLoginButton")
    }
    #endif
}

#Preview {
    LoginView(auth: PrivyAuthService())
}
