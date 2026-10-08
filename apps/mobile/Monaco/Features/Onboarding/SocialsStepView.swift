import MonacoAPI
import MonacoCore
import SwiftUI

struct SocialsStepView: View {
    @Environment(AppEnvironment.self) private var environment

    var mode = LinkStepMode.onboarding
    var onContinue: () -> Void = {}

    var body: some View {
        SocialsStepForm(
            mode: mode,
            model: XLinkModel(
                linking: environment.linking, onboarding: OnboardingAPI(api: environment.api), clock: ContinuousClock()),
            onContinue: onContinue,
            onSignOut: { await environment.signOut() })
    }
}

private struct SocialsStepForm: View {
    @Environment(AppSessionStore.self) private var session
    @Environment(ToastCenter.self) private var toasts
    @Environment(\.dismiss) private var dismiss

    let mode: LinkStepMode
    let onContinue: () -> Void
    let onSignOut: () async -> Void

    @State private var model: XLinkModel

    init(
        mode: LinkStepMode, model: @autoclosure () -> XLinkModel, onContinue: @escaping () -> Void,
        onSignOut: @escaping () async -> Void
    ) {
        self.mode = mode
        self.onContinue = onContinue
        self.onSignOut = onSignOut
        _model = State(wrappedValue: model())
    }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                    Text(LinkCopy.xTitle)
                        .font(MonacoTheme.Typo.display)
                        .foregroundStyle(MonacoTheme.ink)
                        .accessibilityAddTraits(.isHeader)
                    Text(LinkCopy.xSubtext)
                        .font(MonacoTheme.Typo.callout)
                        .foregroundStyle(MonacoTheme.secondaryText)
                }
                .fixedSize(horizontal: false, vertical: true)

                if case .error(let text) = model.caption {
                    Text(text)
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.loss)
                        .fixedSize(horizontal: false, vertical: true)
                        .accessibilityIdentifier("socials-step-caption")
                }
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.top, MonacoTheme.Space.xl)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .safeAreaInset(edge: .bottom) { BottomCTA { primaryButton } }
        .monacoCanvas()
        .navigationTitle("")
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            FirstRunToolbar(
                signOut: mode == .onboarding
                    ? FirstRunAction(
                        title: OnboardingCopy.signOut, identifier: "onboarding-socials-step-sign-out",
                        isDisabled: model.isBusy
                    ) { Task { await onSignOut() } } : nil,
                skip: model.connectHidden
                    ? nil
                    : FirstRunAction(title: skipTitle, identifier: "socials-step-skip", isDisabled: model.isBusy) {
                        Task { await skip() }
                    })
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("onboarding-socials-step")
    }

    @ViewBuilder
    private var primaryButton: some View {
        if model.connectHidden {
            Button {
                Task { await skip() }
            } label: {
                Text(skipTitle).frame(maxWidth: .infinity)
            }
            .buttonStyle(.monacoPrimary)
            .disabled(model.isBusy)
            .accessibilityIdentifier("socials-step-skip")
        } else {
            Button {
                Task { handle(await model.connect()) }
            } label: {
                SubmitLabel(
                    isWorking: model.activity == .connecting, idle: LinkCopy.connectX, working: LinkCopy.connecting)
            }
            .buttonStyle(.monacoPrimary)
            .disabled(model.isBusy)
            .accessibilityIdentifier("socials-step-connect")
        }
    }

    private var skipTitle: String { mode == .sheet ? LinkCopy.notNow : LinkCopy.skip }

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
                toasts.show(success: LinkCopy.xConnected)
                dismiss()
            }
        }
    }
}
