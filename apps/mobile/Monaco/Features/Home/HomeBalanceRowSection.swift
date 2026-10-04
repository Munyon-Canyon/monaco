import MonacoAPI
import MonacoCore
import SwiftUI

enum HomeBalanceDisplay: Equatable {
    case loading
    case amount(Int64)
    case unavailable

    static func resolve(_ state: LoadState<AccountBalance>) -> HomeBalanceDisplay {
        switch state {
        case .idle, .loading: .loading
        case .loaded(let balance): .amount(balance.availableMicros)
        case .failed: .unavailable
        }
    }
}

struct HomeBalanceRowSection: View {
    var identifierPrefix = "home"
    var balanceIdentifier = "platform-balance-value"

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(ScreenRefresh.self) private var refresh: ScreenRefresh?
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @State private var model: BalanceSource?

    var body: some View {
        MonacoGroupedList {
            PlatformBalanceCard(state: model?.state ?? .loading, valueIdentifier: balanceIdentifier)
            actions
                .padding(.leading, MonacoTheme.Space.m + 44 + MonacoTheme.Space.sm)
                .padding(.trailing, MonacoTheme.Space.m)
                .padding(.bottom, MonacoTheme.Space.xs)
        }
        .task {
            let model = preparedModel()
            refresh?.register("balance") { await model.load() }
            await model.load()
            await model.observe()
        }
        .onScreenVisibilityChange { visible in
            model?.setVisible(visible)
        }
        .onChange(of: model?.failureTick) { _, _ in
            guard let error = model?.lastError else { return }
            toasts.current = MonacoToast(message: BalanceSource.message(for: error))
        }
    }

    @ViewBuilder
    private var actions: some View {
        if dynamicTypeSize.isAccessibilitySize {
            VStack(alignment: .leading, spacing: 0) {
                link("Add money", DepositRoute(), id: "\(identifierPrefix)-add-money-link")
                link("Withdraw", WithdrawRoute(), id: "\(identifierPrefix)-withdraw-link")
            }
        } else {
            HStack(spacing: MonacoTheme.Space.l) {
                link("Add money", DepositRoute(), id: "\(identifierPrefix)-add-money-link")
                link("Withdraw", WithdrawRoute(), id: "\(identifierPrefix)-withdraw-link")
                Spacer(minLength: 0)
            }
        }
    }

    private func link(_ title: String, _ route: some AppRoute, id: String) -> some View {
        NavigationLink(value: AnyAppRoute(route)) {
            Text(title)
                .font(MonacoTheme.Typo.calloutStrong)
                .foregroundStyle(MonacoTheme.brand)
                .frame(minHeight: 44)
                .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityIdentifier(id)
    }

    private func preparedModel() -> BalanceSource {
        if let model { return model }
        let created = BalanceSource(api: environment.api, hints: environment.hints)
        model = created
        return created
    }
}
