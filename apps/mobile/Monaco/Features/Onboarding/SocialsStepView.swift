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
                        .font(MonacoTheme.Typo.title)
                        .foregroundStyle(MonacoTheme.primaryText)
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
        .safeAreaInset(edge: .bottom) { actions }
        .monacoCanvas()
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("onboarding-socials-step")
    }

    private var actions: some View {
        VStack(spacing: MonacoTheme.Space.xs) {
            if !model.linkedElsewhere {
                Button {
                    Task { handle(await model.connect()) }
                } label: {
                    HStack(spacing: MonacoTheme.Space.s) {
                        if model.activity == .connecting {
                            ProgressView().tint(MonacoTheme.primaryButtonLabel)
                            Text(LinkCopy.connecting)
                        } else {
                            Text(LinkCopy.connectX)
                        }
                    }
                    .frame(maxWidth: .infinity)
                }
                .buttonStyle(.monacoPrimary)
                .disabled(model.isBusy)
                .accessibilityIdentifier("socials-step-connect")
            }
            skipButton
            if mode == .onboarding {
                Button {
                    Task { await onSignOut() }
                } label: {
                    Text(OnboardingCopy.signOut)
                        .font(MonacoTheme.Typo.calloutStrong)
                        .frame(minHeight: 44)
                        .contentShape(Rectangle())
                }
                .buttonStyle(LinkTextActionStyle())
                .disabled(model.isBusy)
                .accessibilityIdentifier("onboarding-socials-step-sign-out")
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.vertical, MonacoTheme.Space.s)
        .background(MonacoTheme.canvas)
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
            .accessibilityIdentifier("socials-step-skip")
        } else {
            Button {
                Task { await skip() }
            } label: {
                Text(title)
                    .font(MonacoTheme.Typo.calloutStrong)
                    .frame(minHeight: 44)
                    .contentShape(Rectangle())
            }
            .buttonStyle(LinkTextActionStyle())
            .disabled(model.isBusy)
            .accessibilityIdentifier("socials-step-skip")
        }
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
                toasts.show(success: LinkCopy.xConnected)
                dismiss()
            }
        }
    }
}
