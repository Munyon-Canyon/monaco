import MonacoCore
import SwiftUI

struct RestrictedAccountView: View {
    @Environment(AppEnvironment.self) private var environment
    @State private var path: [AnyAppRoute] = []

    var body: some View {
        NavigationStack(path: $path) {
            VStack(spacing: MonacoTheme.Space.m) {
                Spacer()
                Text(OnboardingCopy.restrictedTitle)
                    .font(MonacoTheme.Typo.title)
                    .foregroundStyle(MonacoTheme.primaryText)
                    .multilineTextAlignment(.center)
                    .fixedSize(horizontal: false, vertical: true)
                    .accessibilityAddTraits(.isHeader)
                Text(OnboardingCopy.restrictedBody)
                    .font(MonacoTheme.Typo.body)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .multilineTextAlignment(.center)
                    .fixedSize(horizontal: false, vertical: true)
                Spacer()
                Button {
                    path.append(AnyAppRoute(WithdrawRoute()))
                } label: {
                    Text(OnboardingCopy.withdraw).frame(maxWidth: .infinity)
                }
                .buttonStyle(.monacoPrimary)
                .accessibilityIdentifier("restricted-withdraw")
                Button {
                    path.append(AnyAppRoute(RestrictedCabalsRoute()))
                } label: {
                    Text(OnboardingCopy.cashOut).frame(maxWidth: .infinity)
                }
                .buttonStyle(.monacoSecondary)
                .accessibilityIdentifier("restricted-cash-out")
                textLink(OnboardingCopy.deleteAccount, identifier: "restricted-delete-account") {
                    path.append(AnyAppRoute(DeleteAccountRoute()))
                }
                textLink(OnboardingCopy.signOut, identifier: "restricted-sign-out") {
                    Task { await environment.signOut() }
                }
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.bottom, MonacoTheme.Space.m)
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .monacoCanvas()
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier("restrictedAccountView")
            .navigationDestination(for: AnyAppRoute.self) { $0.destination() }
        }
    }

    private func textLink(_ title: String, identifier: String, action: @escaping () -> Void) -> some View {
        Button(action: action) {
            Text(title)
                .font(MonacoTheme.Typo.calloutStrong)
                .foregroundStyle(MonacoTheme.brand)
                .frame(minHeight: 44)
                .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityIdentifier(identifier)
    }
}

private nonisolated struct RestrictedCabalsRoute: AppRoute {
    @MainActor func destination() -> some View {
        CabalsTab.root()
            .environment(\.accountRestricted, true)
    }
}
