import MonacoAPI
import MonacoCore
import SwiftUI

enum LinkStepMode {
    case onboarding
    case sheet
}

enum PhoneStepPrimary: Equatable {
    case skip
    case link
    case sendCode

    static func resolve(linkedElsewhere: Bool, isCodeStep: Bool) -> PhoneStepPrimary {
        if linkedElsewhere { return .skip }
        return isCodeStep ? .link : .sendCode
    }

    var showsToolbarSkip: Bool { self != .skip }
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
                if isCodeStep { codeActions }
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.top, MonacoTheme.Space.xl)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .scrollDismissesKeyboard(.interactively)
        .safeAreaInset(edge: .bottom) { BottomCTA { primaryButton } }
        .monacoCanvas()
        .navigationTitle("")
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            FirstRunToolbar(
                signOut: mode == .onboarding
                    ? FirstRunAction(
                        title: OnboardingCopy.signOut, identifier: "onboarding-phone-step-sign-out",
                        isDisabled: model.isBusy
                    ) { Task { await onSignOut() } } : nil,
                skip: primaryAction.showsToolbarSkip
                    ? FirstRunAction(title: skipTitle, identifier: "phone-step-skip", isDisabled: model.isBusy) {
                        Task { await skip() }
                    } : nil)
        }
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

    private var primaryAction: PhoneStepPrimary {
        PhoneStepPrimary.resolve(linkedElsewhere: model.linkedElsewhere, isCodeStep: isCodeStep)
    }

    @ViewBuilder
    private var primaryButton: some View {
        switch primaryAction {
        case .skip:
            skipPrimary
        case .link:
            Button {
                Task { await link() }
            } label: {
                SubmitLabel(
                    isWorking: model.activity == .linking, idle: OnboardingCopy.continueLabel,
                    working: LinkCopy.linking)
            }
            .buttonStyle(.monacoPrimary)
            .disabled(!model.canLink)
            .accessibilityIdentifier("phone-step-continue")
        case .sendCode:
            Button {
                Task { await sendCode() }
            } label: {
                SubmitLabel(
                    isWorking: model.activity == .sending, idle: LinkCopy.sendCode, working: LinkCopy.sendingCode)
            }
            .buttonStyle(.monacoPrimary)
            .disabled(e164 == nil || model.isBusy)
            .accessibilityIdentifier("phone-step-send-code")
        }
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

    private var skipTitle: String { mode == .sheet ? LinkCopy.notNow : LinkCopy.skip }

    @ViewBuilder
    private var skipPrimary: some View {
        Button {
            Task { await skip() }
        } label: {
            Text(skipTitle).frame(maxWidth: .infinity)
        }
        .buttonStyle(.monacoPrimary)
        .disabled(model.isBusy)
        .accessibilityIdentifier("phone-step-skip")
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
