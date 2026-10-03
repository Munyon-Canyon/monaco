import MonacoCore
import SwiftUI

struct OnboardingStepScaffold: View {
    @Environment(AppEnvironment.self) private var environment

    let title: String
    let identifier: String
    let onContinue: () -> Void

    var body: some View {
        VStack(spacing: MonacoTheme.Space.l) {
            Spacer()
            Text(title)
                .font(MonacoTheme.Typo.title)
                .foregroundStyle(MonacoTheme.primaryText)
                .multilineTextAlignment(.center)
                .fixedSize(horizontal: false, vertical: true)
                .accessibilityAddTraits(.isHeader)
            Spacer()
            Button(action: onContinue) {
                Text(OnboardingCopy.continueLabel)
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(.monacoPrimary)
            .accessibilityIdentifier("\(identifier)-continue")
            Button {
                Task { await environment.signOut() }
            } label: {
                Text(OnboardingCopy.signOut)
                    .font(MonacoTheme.Typo.calloutStrong)
                    .foregroundStyle(MonacoTheme.brand)
                    .frame(minHeight: 44)
                    .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityIdentifier("\(identifier)-sign-out")
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.bottom, MonacoTheme.Space.m)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .monacoCanvas()
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier(identifier)
    }
}
