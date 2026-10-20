import MonacoCore
import SwiftUI
import UIKit

/// What a one-time-code form asks for before it can send a code. `.sms` and `.email` are defined
/// next to their views.
/// Which of the service's two one-time-code paths a destination goes through.
enum OTPChannel {
    case sms
    case email
}

struct OTPDestination {
    let channel: OTPChannel
    /// Under the field, always: what the button will do.
    let caption: String
    let prompt: String
    let placeholder: String
    let keyboardType: UIKeyboardType
    let contentType: UITextContentType
    /// Shown under the field once what's typed can't be sent to.
    let invalidHint: String
    let changeLabel: String
    let verifyTitle: String
    let addressFieldIdentifier: String
    let identifierPrefix: String
    /// The address in the shape the provider needs, or nil when it isn't one yet.
    let normalize: (String) -> String?
    /// How the address reads back under "Enter the 6-digit code sent to".
    let display: (String) -> String
}

/// The code's rules and look, shared by both methods.
enum OTPCode {
    static let length = 6
    /// Wide, so six digits read as a code to copy across rather than as an amount.
    static let tracking: CGFloat = 6
    /// The button and the text actions: what the form keeps above the keyboard.
    static let actionsAnchor = "otp-login-actions"

    /// The provider turned the code down. The one failure that is about what is in the field.
    static var rejectedMessage: String {
        LoginFailureCopy.message(for: .codeRejected, step: .verifyCode)
    }

    static var offlineMessage: String {
        LoginFailureCopy.message(for: .offline, step: .sendCode)
    }
}

extension LoginFlow {
    var isOnCodeStep: Bool {
        if case .authenticated = phase { return true }
        return isCodeEntry
    }

    var isOffline: Bool {
        phase == .failed(message: OTPCode.offlineMessage)
    }
}

/// What the form says besides its helper: an error or an in-flight note as a notice, an address
/// hint as the line above the helper. One slot, so two notices never stack up.
enum OTPFieldCaption: Equatable {
    /// The last request failed. Set in `loss`.
    case error(String)
    /// What's typed can't be sent to yet.
    case hint(String)
    /// What is happening, or just happened.
    case note(String)

    static let sendingCode = "Sending your sign-in code\u{2026}"
    static let verifyingCode = "Verifying your code\u{2026}"
    static let sendingNewCode = "Sending a new code\u{2026}"
    static let newCodeSent = "New code sent."

    var text: String {
        switch self {
        case .error(let text), .hint(let text), .note(let text): return text
        }
    }

    var tone: MonacoNotice.Tone? {
        switch self {
        case .error: return .error
        case .note: return .info
        case .hint: return nil
        }
    }

    static func resolve(
        phase: LoginPhase,
        isCodeStep: Bool,
        invalidHint: String?,
        resent: Bool
    ) -> OTPFieldCaption? {
        if case .failed(let message) = phase { return .error(message) }
        if isCodeStep {
            switch phase {
            case .verifyingCode: return .note(verifyingCode)
            case .sendingCode: return .note(sendingNewCode)
            default: return resent ? .note(newCodeSent) : nil
            }
        }
        if phase == .sendingCode { return .note(sendingCode) }
        return invalidHint.map { .hint($0) }
    }
}

/// The button's words: what it will do, or what it is doing.
enum OTPPrimaryAction {
    static func title(phase: LoginPhase, isCodeStep: Bool) -> String {
        if isCodeStep {
            // A resend in flight is not the member signing in: the button keeps its name.
            return phase == .verifyingCode ? "Verifying\u{2026}" : "Continue"
        }
        return phase == .sendingCode ? "Sending\u{2026}" : "Send code"
    }
}

/// One-time-code sign-in: an address, then the code, then in. SMS and email differ only in the
/// address they ask for, so both run through this one form, and the code rules (6 digits, digits
/// only, auto-submit) live in a single place.
///
/// The address step is a labelled field, a helper and the button. The code step is its own screen:
/// a back button, the boxes and the button, with "Send a new code" and "Change number" as text
/// under it. When the connection is down either step gives way to a "Try again" empty state.
struct OTPLoginForm: View {
    @ObservedObject var auth: PrivyAuthService
    let destination: OTPDestination
    /// The login screen's scroll view, so the button can be kept above the keyboard.
    let scroll: ScrollViewProxy

    @State private var address: String
    @State private var otpCode: String
    /// A new code went out after the first one. Said once under the boxes, until the member types.
    @State private var resent = false
    @State private var cooldown = ResendCooldown(clock: ContinuousClock())
    @State private var addressHintArmed: Bool
    @FocusState private var focusedField: Field?
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @Environment(\.analytics) private var analytics
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    private enum Field {
        case address
        case code
    }

    init(
        auth: PrivyAuthService,
        destination: OTPDestination,
        scroll: ScrollViewProxy,
        initialCode: String = "",
        initialAddress: String = ""
    ) {
        self.auth = auth
        self.destination = destination
        self.scroll = scroll
        _otpCode = State(initialValue: initialCode)
        _address = State(initialValue: initialAddress)
        _addressHintArmed = State(
            initialValue: !initialAddress.isEmpty && destination.normalize(initialAddress) == nil)
    }

    // The form calls the service itself rather than through closures the two login views
    // handed it: the stored `(String) async -> Void` went through reabstraction thunks that
    // crashed with a bus error in `swift_retain` on the first send.
    private func send(_ address: String) async {
        analytics.loginStarted()
        switch destination.channel {
        case .sms: await auth.sendSMSCode(to: address)
        case .email: await auth.sendEmailCode(to: address)
        }
    }

    private func verify(_ code: String, sentTo address: String) async {
        switch destination.channel {
        case .sms: await auth.loginWithSMSCode(code, sentTo: address)
        case .email: await auth.loginWithEmailCode(code, sentTo: address)
        }
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            if isCodeStep {
                VerifyCodeHeader(
                    backIdentifier: "\(destination.identifierPrefix)VerifyBackButton",
                    onBack: changeAddress
                )
                .disabled(auth.flow.isBusy)
                .authStepTransition(reduceMotion: reduceMotion)
            }
            if auth.flow.isOffline {
                offlineState
            } else if isCodeStep {
                verifyStep
            } else {
                addressStep
            }
        }
        .monacoPrivate()
        .pausesSessionReplay(isCodeStep)
        .revealsWhileActive(
            OTPCode.actionsAnchor,
            isActive: focusedField != nil,
            tracking: revealKey,
            proxy: scroll
        )
        .onChange(of: auth.flow.phase) { _, phase in
            guard case .failed(let message) = phase else { return }
            if message == OTPCode.rejectedMessage {
                otpCode = ""
            }
            Haptics.warning()
            AccessibilityNotification.Announcement(message).post()
        }
        .onChange(of: caption) { _, latest in
            guard case .note(let text)? = latest else { return }
            AccessibilityNotification.Announcement(text).post()
        }
        .onChange(of: isCodeStep) { _, onCodeStep in
            if onCodeStep { restartCooldown() }
        }
        .onChange(of: address) {
            addressHintArmed = false
            auth.clearLoginFailure()
        }
        .onChange(of: focusedField) { old, _ in
            if old == .address, focusedField != .address,
                !address.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty,
                destination.normalize(address) == nil
            {
                addressHintArmed = true
            }
        }
    }
}

extension OTPLoginForm {
    // MARK: Steps

    private var addressStep: some View {
        VStack(alignment: .leading, spacing: 0) {
            Text(destination.prompt)
                .font(MonacoTheme.Typo.captionStrong)
                .foregroundStyle(MonacoTheme.ink)
                .padding(.bottom, MonacoTheme.Space.s)
                .accessibilityHidden(true)
            addressField
            hintLine
            Text(destination.caption)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
                .fixedSize(horizontal: false, vertical: true)
                .padding(.top, MonacoTheme.Space.s)
            noticeLine(after: MonacoTheme.Space.sm)
            actions
        }
        .authStepTransition(reduceMotion: reduceMotion)
    }

    private var verifyStep: some View {
        VStack(alignment: .leading, spacing: 0) {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
                Text(destination.verifyTitle)
                    .authHeadline()
                if let sentTo = auth.flow.destination {
                    sentLine(sentTo)
                }
            }
            .padding(.top, MonacoTheme.Space.xl)
            codeField
                .padding(.top, MonacoTheme.Space.l)
            noticeLine(after: MonacoTheme.Space.m)
            actions
        }
        .authStepTransition(reduceMotion: reduceMotion)
    }

    private var offlineState: some View {
        EmptyState(
            title: "Couldn't load this view",
            message: "Your existing information hasn't been changed. Try again when your connection is available.",
            actionTitle: "Try again",
            systemImage: MonacoIcon.offline.symbol,
            action: retryAfterOffline
        )
        .authStepTransition(reduceMotion: reduceMotion)
    }

    // MARK: Pieces

    private func sentLine(_ sentTo: String) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            Text("Enter the \(OTPCode.length)-digit code sent to")
                .font(MonacoTheme.Typo.body)
            Text(destination.display(sentTo))
                .font(MonacoTheme.Typo.headline)
        }
        .foregroundStyle(MonacoTheme.muted)
        .fixedSize(horizontal: false, vertical: true)
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityElement(children: .combine)
    }

    private var addressField: some View {
        TextField(
            destination.prompt,
            text: $address,
            prompt: Text(destination.placeholder).foregroundStyle(MonacoTheme.muted)
        )
        .keyboardType(destination.keyboardType)
        .textContentType(destination.contentType)
        .textInputAutocapitalization(.never)
        .autocorrectionDisabled()
        .submitLabel(.send)
        .onSubmit {
            Task { await sendCode() }
        }
        .focused($focusedField, equals: .address)
        .authTextFieldStyle(
            isFocused: focusedField == .address,
            isInvalid: showsAddressHint,
            focus: { focusedField = .address }
        )
        .accessibilityIdentifier(destination.addressFieldIdentifier)
    }

    private var codeField: some View {
        OTPCodeField(
            code: $otpCode,
            isFocused: focusedField == .code,
            isInvalid: codeWasRejected,
            identifier: "\(destination.identifierPrefix)CodeField",
            focus: { focusedField = .code }
        ) { code in
            // Autofill drops all six digits in at once: don't make them tap Continue too.
            guard !auth.flow.isBusy else { return }
            Task { await submitCode(code) }
        }
        .focused($focusedField, equals: .code)
        .onChange(of: otpCode) { _, newValue in
            if !newValue.isEmpty {
                resent = false
                if auth.pendingUser == nil {
                    auth.clearLoginFailure()
                }
            }
        }
    }

    @ViewBuilder
    private var hintLine: some View {
        if case .hint(let text) = caption {
            Text(text)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.loss)
                .fixedSize(horizontal: false, vertical: true)
                .padding(.top, MonacoTheme.Space.xs)
                .accessibilityIdentifier("\(destination.identifierPrefix)AddressHint")
        }
    }

    @ViewBuilder
    private func noticeLine(after spacing: CGFloat) -> some View {
        if let caption, let tone = caption.tone {
            MonacoNotice(tone, message: caption.text)
                .padding(.top, spacing)
        }
    }

    private var actions: some View {
        VStack(alignment: .leading, spacing: 0) {
            primaryButton
            if isCodeStep {
                codeActions
                    .padding(.top, MonacoTheme.Space.m)
            }
        }
        .padding(.top, MonacoTheme.Space.l)
        .id(OTPCode.actionsAnchor)
    }

    @ViewBuilder
    private var primaryButton: some View {
        let isWorking = auth.flow.phase == (isCodeStep ? .verifyingCode : .sendingCode)
        if isCodeStep {
            Button {
                Task { await submitCode(otpCode) }
            } label: {
                SubmitLabel(
                    isWorking: isWorking,
                    idle: auth.pendingUser == nil
                        ? OTPPrimaryAction.title(phase: .idle, isCodeStep: true) : "Try again",
                    working: OTPPrimaryAction.title(phase: .verifyingCode, isCodeStep: true))
            }
            .buttonStyle(.monacoPrimary)
            .disabled(isVerifyDisabled)
            .accessibilityIdentifier("\(destination.identifierPrefix)VerifyButton")
        } else {
            Button {
                Task { await sendCode() }
            } label: {
                SubmitLabel(
                    isWorking: isWorking,
                    idle: OTPPrimaryAction.title(phase: .idle, isCodeStep: false),
                    working: OTPPrimaryAction.title(phase: .sendingCode, isCodeStep: false))
            }
            .buttonStyle(.monacoPrimary)
            .disabled(isSendDisabled)
            .accessibilityIdentifier("\(destination.identifierPrefix)SendCodeButton")
        }
    }

    /// The two ends sit apart, as in the design; one under the other at the accessibility sizes,
    /// where two labels in a row run into each other.
    private var codeActions: some View {
        let isStacked = dynamicTypeSize.isAccessibilitySize
        let layout =
            isStacked
            ? AnyLayout(VStackLayout(alignment: .leading, spacing: 0))
            : AnyLayout(HStackLayout(spacing: MonacoTheme.Space.m))
        return layout {
            TimelineView(.periodic(from: .now, by: 1)) { _ in
                textAction(cooldown.label, identifier: "\(destination.identifierPrefix)ResendCodeButton") {
                    Task { await resendCode() }
                }
                .disabled(!cooldown.canResend || auth.flow.isBusy)
            }
            if !isStacked {
                Spacer(minLength: 0)
            }
            textAction(destination.changeLabel, identifier: "\(destination.identifierPrefix)ChangeAddressButton") {
                changeAddress()
            }
        }
    }

    private func textAction(_ title: String, identifier: String, action: @escaping () -> Void) -> some View {
        Button(action: action) {
            Text(title)
        }
        .buttonStyle(.monacoText)
        .disabled(auth.flow.isBusy)
        .accessibilityIdentifier(identifier)
    }
}

extension OTPLoginForm {
    // MARK: Actions

    private func sendCode() async {
        guard let normalized = destination.normalize(address) else {
            addressHintArmed = true
            return
        }
        await send(normalized)
        if case .awaitingCode = auth.flow.phase {
            focusedField = .code
        }
    }

    /// Keeps whatever the member has typed unless the new code actually went out: a throttled
    /// resend used to wipe the field it was about to be typed into.
    private func resendCode() async {
        guard let sentTo = sentDestination else { return }
        await send(sentTo)
        if case .awaitingCode = auth.flow.phase {
            otpCode = ""
            resent = true
            restartCooldown()
            focusedField = .code
        }
    }

    private func changeAddress() {
        otpCode = ""
        resent = false
        auth.resetLoginFlow()
        // Next turn of the run loop: the address field is only back on screen after this update.
        DispatchQueue.main.async {
            focusedField = .address
        }
    }

    private func retryAfterOffline() {
        guard isCodeStep else {
            auth.clearLoginFailure()
            return
        }
        otpCode = ""
        auth.clearLoginFailure()
        DispatchQueue.main.async {
            focusedField = .code
        }
    }

    private func submitCode(_ code: String) async {
        if auth.pendingUser != nil {
            guard !auth.flow.isBusy else { return }
            await auth.retryPendingSignIn()
            return
        }
        guard code.count == OTPCode.length, !auth.flow.isBusy, let sentTo = sentDestination else { return }
        await verify(code, sentTo: sentTo)
    }
}

extension OTPLoginForm {
    private func restartCooldown() {
        cooldown = ResendCooldown(clock: ContinuousClock())
        cooldown.restart()
    }

    // MARK: Derived state

    /// The address the code actually went to, not whatever is in the field now.
    private var sentDestination: String? {
        auth.flow.destination ?? destination.normalize(address)
    }

    private var isCodeStep: Bool { auth.flow.isOnCodeStep }

    private var showsAddressHint: Bool {
        !isCodeStep
            && addressHintArmed
            && !address.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
            && destination.normalize(address) == nil
    }

    private var isSendDisabled: Bool {
        destination.normalize(address) == nil || auth.flow.isBusy
    }

    private var isVerifyDisabled: Bool {
        (auth.pendingUser == nil && otpCode.count != OTPCode.length) || auth.flow.isBusy
    }

    /// Only a turned-down code marks the field; "no connection" is not about what is in it.
    private var codeWasRejected: Bool {
        auth.flow.phase == .failed(message: OTPCode.rejectedMessage)
    }

    private var caption: OTPFieldCaption? {
        OTPFieldCaption.resolve(
            phase: auth.flow.phase,
            isCodeStep: isCodeStep,
            invalidHint: showsAddressHint ? destination.invalidHint : nil,
            resent: resent
        )
    }

    /// Changes whenever the block under the field changes height, so it is revealed again.
    private var revealKey: String {
        "\(isCodeStep)|\(caption?.text ?? "")"
    }
}
