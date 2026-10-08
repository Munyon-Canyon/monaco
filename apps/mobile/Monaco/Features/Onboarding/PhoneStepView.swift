import MonacoAPI
import MonacoCore
import SwiftUI

enum LinkStepMode {
    case onboarding
    case sheet
}

struct PhoneStepView: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(AppSessionStore.self) private var session

    var mode = LinkStepMode.onboarding
    var onContinue: () -> Void = {}
    var linking: (any AccountLinking)?
    var onboarding: OnboardingAPI?

    var body: some View {
        PhoneStepForm(
            mode: mode,
            confirmsSignInPhone: mode == .onboarding && session.profile?.authState == .created
                && session.profile?.loginProvider == .sms,
            model: PhoneLinkModel(
                linking: linking ?? environment.linking,
                onboarding: onboarding ?? OnboardingAPI(api: environment.api),
                clock: ContinuousClock()),
            onContinue: onContinue,
            onSignOut: { await environment.signOut() })
    }
}

private struct PhoneStepForm: View {
    @Environment(AppSessionStore.self) private var session
    @Environment(ToastCenter.self) private var toasts
    @Environment(\.dismiss) private var dismiss
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    let mode: LinkStepMode
    let confirmsSignInPhone: Bool
    let onContinue: () -> Void
    let onSignOut: () async -> Void

    @State private var model: PhoneLinkModel
    @State private var number = ""
    @FocusState private var focused: Field?

    private enum Field {
        case number
        case code
    }

    init(
        mode: LinkStepMode, confirmsSignInPhone: Bool, model: @autoclosure () -> PhoneLinkModel,
        onContinue: @escaping () -> Void,
        onSignOut: @escaping () async -> Void
    ) {
        self.mode = mode
        self.confirmsSignInPhone = confirmsSignInPhone
        self.onContinue = onContinue
        self.onSignOut = onSignOut
        _model = State(wrappedValue: model())
    }

    private var e164: String? { E164PhoneNumber(number)?.value }

    private var isCodeStep: Bool {
        if case .code = model.step { return true }
        return false
    }

    private var isConfirming: Bool { confirmsSignInPhone && !model.signInPhoneUnavailable }

    var body: some View {
        if isConfirming {
            confirmingBody
        } else {
            formBody
        }
    }

    private var confirmingBody: some View {
        PhoneConfirmingSkeleton(subtext: subtext)
            .monacoCanvas()
            .accessibilityIdentifier("onboarding-phone-confirming")
            .task { handle(await model.confirmSignInPhone()) }
    }

    private var formBody: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                    Text(LinkCopy.phoneTitle)
                        .font(MonacoTheme.Typo.display)
                        .foregroundStyle(MonacoTheme.ink)
                        .accessibilityAddTraits(.isHeader)
                    subtext
                }
                .fixedSize(horizontal: false, vertical: true)

                if isCodeStep {
                    OTPCodeField(
                        code: $model.code,
                        isFocused: focused == .code,
                        isInvalid: model.caption == .error(LinkCopy.invalidCode),
                        identifier: "phone-step-code-field"
                    ) { _ in
                        Task { await link() }
                    }
                    .focused($focused, equals: .code)
                } else {
                    numberField
                }

                LinkCaptionLine(caption: model.caption, identifier: "phone-step-caption")
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.top, MonacoTheme.Space.xl)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .scrollDismissesKeyboard(.interactively)
        .safeAreaInset(edge: .bottom) { actions }
        .monacoCanvas()
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("onboarding-phone-step")
        .onAppear { focused = .number }
        .onChange(of: isCodeStep) { _, codeStep in
            focused = codeStep ? .code : .number
        }
    }

    @ViewBuilder
    private var subtext: some View {
        if case .code(let sentTo) = model.step {
            Text(LinkCopy.codeSent(to: PhoneReadBack.format(sentTo)))
                .font(MonacoTheme.Typo.callout)
                .foregroundStyle(MonacoTheme.secondaryText)
                .accessibilityIdentifier("phone-step-sent-to")
        } else {
            Text(LinkCopy.phoneSubtext)
                .font(MonacoTheme.Typo.callout)
                .foregroundStyle(MonacoTheme.secondaryText)
        }
    }

    private var numberField: some View {
        TextField(
            "Phone number", text: $number,
            prompt: Text("Phone number").foregroundStyle(MonacoTheme.disabledLabel)
        )
        .keyboardType(.phonePad)
        .textContentType(.telephoneNumber)
        .submitLabel(.send)
        .onSubmit { Task { await sendCode() } }
        .focused($focused, equals: .number)
        .authTextFieldStyle(isFocused: focused == .number, isInvalid: model.linkedElsewhere)
        .disabled(model.isBusy)
        .accessibilityIdentifier("phone-step-number-field")
    }

    private var actions: some View {
        VStack(spacing: MonacoTheme.Space.xs) {
            primaryButton
            if isCodeStep { codeActions }
            skipButton
            if mode == .onboarding {
                textAction(OnboardingCopy.signOut, identifier: "onboarding-phone-step-sign-out") {
                    Task { await onSignOut() }
                }
                .disabled(model.isBusy)
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.vertical, MonacoTheme.Space.s)
        .background(MonacoTheme.canvas)
    }

    @ViewBuilder
    private var primaryButton: some View {
        if isCodeStep {
            Button {
                Task { await link() }
            } label: {
                busyLabel(model.activity == .linking ? LinkCopy.linking : OnboardingCopy.continueLabel)
            }
            .buttonStyle(.monacoPrimary)
            .disabled(!model.canLink)
            .accessibilityIdentifier("phone-step-continue")
        } else if !model.linkedElsewhere {
            Button {
                Task { await sendCode() }
            } label: {
                busyLabel(model.activity == .sending ? LinkCopy.sendingCode : LinkCopy.sendCode)
            }
            .buttonStyle(.monacoPrimary)
            .disabled(e164 == nil || model.isBusy)
            .accessibilityIdentifier("phone-step-send-code")
        }
    }

    private func busyLabel(_ title: String) -> some View {
        HStack(spacing: MonacoTheme.Space.s) {
            if model.activity == .sending || model.activity == .linking {
                ProgressView().tint(MonacoTheme.primaryButtonLabel)
            }
            Text(title)
        }
        .frame(maxWidth: .infinity)
    }

    private var codeActions: some View {
        let layout =
            dynamicTypeSize.isAccessibilitySize
            ? AnyLayout(VStackLayout(alignment: .leading, spacing: 0))
            : AnyLayout(HStackLayout(spacing: MonacoTheme.Space.l))
        return layout {
            TimelineView(.periodic(from: .now, by: 1)) { _ in
                textAction(model.cooldown.label, identifier: "phone-step-resend") {
                    Task { await model.resend() }
                }
                .disabled(!model.cooldown.canResend || model.isBusy)
            }
            textAction(LinkCopy.changeNumber, identifier: "phone-step-change-number") {
                model.changeNumber()
            }
            .disabled(model.isBusy)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private func textAction(_ title: String, identifier: String, action: @escaping () -> Void) -> some View {
        Button(action: action) {
            Text(title)
        }
        .buttonStyle(.monacoText)
        .accessibilityIdentifier(identifier)
    }

    @ViewBuilder
    private var skipButton: some View {
        let title = mode == .sheet ? LinkCopy.notNow : LinkCopy.skip
        if model.linkedElsewhere {
            Button {
                Task { await skip() }
            } label: {
                Text(title).frame(maxWidth: .infinity)
            }
            .buttonStyle(.monacoPrimary)
            .disabled(model.isBusy)
            .accessibilityIdentifier("phone-step-skip")
        } else {
            Button {
                Task { await skip() }
            } label: {
                Text(title)
            }
            .buttonStyle(.monacoText)
            .disabled(model.isBusy)
            .accessibilityIdentifier("phone-step-skip")
        }
    }

    private func sendCode() async {
        guard let e164 else { return }
        await model.sendCode(to: e164)
    }

    private func link() async {
        handle(await model.link())
    }

    private func skip() async {
        guard mode == .onboarding else {
            dismiss()
            return
        }
        handle(await model.skip())
    }

    private func handle(_ result: LinkStepResult) {
        switch result {
        case .stay:
            break
        case .toast(let message):
            toasts.current = MonacoToast(message: message)
        case .finished(let profile):
            switch mode {
            case .onboarding:
                onContinue()
                session.profile = profile
            case .sheet:
                session.profile = profile
                toasts.show(success: LinkCopy.phoneAdded)
                dismiss()
            }
        }
    }
}

struct LinkCaptionLine: View {
    let caption: LinkStepCaption?
    let identifier: String

    var body: some View {
        switch caption {
        case .error(let text):
            Text(text)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.loss)
                .fixedSize(horizontal: false, vertical: true)
                .accessibilityIdentifier(identifier)
        case .note(let text):
            Text(text)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
                .fixedSize(horizontal: false, vertical: true)
                .accessibilityIdentifier(identifier)
        case nil:
            EmptyView()
        }
    }
}

private struct PhoneConfirmingSkeleton<Subtext: View>: View {
    let subtext: Subtext

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                Text(LinkCopy.phoneTitle)
                    .font(MonacoTheme.Typo.display)
                    .foregroundStyle(MonacoTheme.ink)
                subtext
            }
            .fixedSize(horizontal: false, vertical: true)
            SkeletonBlock(height: MonacoButtonMetrics.minimumHeight, radius: MonacoTheme.Radius.field)
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.xl)
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        .skeleton(true)
    }
}
