import MonacoCore
import SwiftUI

struct NewAccountStepView: View {
    @Environment(AppEnvironment.self) private var environment

    let onConfirm: () -> Void

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                Text(OnboardingCopy.newAccountTitle)
                    .font(MonacoTheme.Typo.display)
                    .foregroundStyle(MonacoTheme.ink)
                    .accessibilityAddTraits(.isHeader)
                Text(OnboardingCopy.newAccountBody)
                    .font(MonacoTheme.Typo.callout)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .accessibilityIdentifier("new-account-step-body")
            }
            .fixedSize(horizontal: false, vertical: true)
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.top, MonacoTheme.Space.xl)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .safeAreaInset(edge: .bottom) {
            BottomCTA {
                VStack(spacing: MonacoTheme.Space.xs) {
                    Button(action: onConfirm) {
                        Text(OnboardingCopy.createNewAccount)
                    }
                    .buttonStyle(.monacoPrimary)
                    .accessibilityIdentifier("new-account-step-create")
                    Button {
                        Task { await environment.signOut() }
                    } label: {
                        Text(OnboardingCopy.signInAnotherWay)
                    }
                    .buttonStyle(.monacoText)
                    .accessibilityIdentifier("new-account-step-sign-in")
                }
            }
        }
        .monacoCanvas()
        .navigationTitle("")
        .navigationBarTitleDisplayMode(.inline)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("onboarding-new-account-step")
    }
}
