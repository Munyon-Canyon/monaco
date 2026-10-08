import MonacoCore
import SwiftUI

struct RestrictedAccountView: View {
    @Environment(AppEnvironment.self) private var environment
    @State private var path: [AnyAppRoute] = []

    var body: some View {
        NavigationStack(path: $path) {
            ScrollView {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                    Text(OnboardingCopy.restrictedTitle)
                        .font(MonacoTheme.Typo.display)
                        .foregroundStyle(MonacoTheme.ink)
                        .accessibilityAddTraits(.isHeader)
                    Text(OnboardingCopy.restrictedBody)
                        .font(MonacoTheme.Typo.callout)
                        .foregroundStyle(MonacoTheme.secondaryText)
                }
                .fixedSize(horizontal: false, vertical: true)
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .padding(.top, MonacoTheme.Space.xl)
                .frame(maxWidth: .infinity, alignment: .leading)
            }
            .safeAreaInset(edge: .bottom) {
                BottomCTA {
                    VStack(spacing: MonacoTheme.Space.xs) {
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
                }
            }
            .monacoCanvas()
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier("restrictedAccountView")
            .navigationDestination(for: AnyAppRoute.self) { $0.destination() }
        }
    }

    private func textLink(_ title: String, identifier: String, action: @escaping () -> Void) -> some View {
        Button(action: action) {
            Text(title)
        }
        .buttonStyle(.monacoText)
        .accessibilityIdentifier(identifier)
    }
}

private nonisolated struct RestrictedCabalsRoute: AppRoute {
    @MainActor func destination() -> some View {
        CabalsTab.root()
            .environment(\.accountRestricted, true)
    }
}
