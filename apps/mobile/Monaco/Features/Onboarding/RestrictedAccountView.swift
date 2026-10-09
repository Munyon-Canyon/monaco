import MonacoAPI
import MonacoCore
import SwiftUI

struct RestrictedAccountView: View {
    @Environment(AppEnvironment.self) private var environment

    var body: some View {
        NavigationStack {
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
                        NavigationLink(value: AnyAppRoute(WithdrawRoute())) {
                            Text(OnboardingCopy.withdraw)
                        }
                        .buttonStyle(.monacoPrimary)
                        .accessibilityIdentifier("restricted-withdraw")
                        NavigationLink(value: AnyAppRoute(RestrictedCashOutRoute())) {
                            Text(OnboardingCopy.cashOut)
                        }
                        .buttonStyle(.monacoSecondary)
                        .accessibilityIdentifier("restricted-cash-out")
                        NavigationLink(value: AnyAppRoute(DeleteAccountRoute())) {
                            Text(OnboardingCopy.deleteAccount)
                        }
                        .buttonStyle(.monacoText)
                        .accessibilityIdentifier("restricted-delete-account")
                        Button {
                            Task { await environment.signOut() }
                        } label: {
                            Text(OnboardingCopy.signOut)
                        }
                        .buttonStyle(.monacoText)
                        .accessibilityIdentifier("restricted-sign-out")
                    }
                }
            }
            .monacoCanvas()
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier("restrictedAccountView")
            .navigationDestination(for: AnyAppRoute.self) { $0.destination() }
        }
    }
}

private nonisolated struct RestrictedCashOutRoute: AppRoute {
    @MainActor func destination() -> some View {
        RestrictedCashOutView()
    }
}

private struct RestrictedCashOutView: View {
    @Environment(AppEnvironment.self) private var environment
    @State private var model: DeleteAccountModel?

    var body: some View {
        ScrollView {
            content
                .padding(.vertical, MonacoTheme.Space.m)
        }
        .monacoCanvas()
        .navigationTitle(OnboardingCopy.cashOut)
        .navigationBarTitleDisplayMode(.inline)
        .onAppear {
            let model = preparedModel()
            Task { await model.load() }
        }
    }

    @ViewBuilder private var content: some View {
        switch model?.state ?? .loading {
        case .idle, .loading:
            MonacoRowSkeleton(rows: 2, markShape: .tile)
        case .failed:
            MonacoErrorRow(thing: "your cabals", identifier: "restricted-cash-out-error") {
                Task { await model?.load() }
            }
        case .loaded(let checklist):
            if checklist.slices.isEmpty {
                EmptyState(title: AccountCopy.noCabalMoney)
            } else {
                CabalSliceList(slices: checklist.slices)
            }
        }
    }

    private func preparedModel() -> DeleteAccountModel {
        if let model { return model }
        let created = DeleteAccountModel(api: environment.api)
        model = created
        return created
    }
}
