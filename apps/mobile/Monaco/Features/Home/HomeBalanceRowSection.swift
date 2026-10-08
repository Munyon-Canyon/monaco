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
    @Environment(\.scenePhase) private var scenePhase
    @State private var model: BalanceSource?

    static func showsRetryRow(_ state: LoadState<AccountBalance>) -> Bool {
        if case .failed = state { return true }
        return false
    }

    var body: some View {
        MonacoGroupedList {
            if let model, Self.showsRetryRow(model.state) {
                retryRow(model)
            } else {
                PlatformBalanceCard(
                    state: model?.state ?? .loading, cardProcessing: environment.cardDeposit.isProcessing,
                    valueIdentifier: balanceIdentifier)
            }
            actions
                .padding(.leading, PlatformBalanceCard.leadingInset)
                .padding(.trailing, MonacoTheme.Space.gutter)
                .padding(.bottom, MonacoTheme.Space.sm)
        }
        .task {
            let model = preparedModel()
            refresh?.register("balance") { await model.load() }
            await model.load()
            await model.observe()
        }
        .task {
            await environment.cardDeposit.observe()
        }
        .onScreenVisibilityChange { visible in
            model?.setVisible(visible)
        }
        .onChange(of: scenePhase) { _, phase in
            guard phase == .active else { return }
            Task { await environment.cardDeposit.foregrounded() }
        }
        .onChange(of: model?.failureTick) { _, _ in
            guard model?.balance != nil, let error = model?.lastError else { return }
            toasts.current = MonacoToast(message: BalanceSource.message(for: error))
        }
        .onChange(of: model?.balance) { previous, current in
            guard let current else { return }
            let change = BalanceChange.detect(previous: previous, current: current)
            guard let change else { return }
            environment.cardDeposit.balanceChanged(change)
            toasts.show(success: change.message)
        }
    }

    private func retryRow(_ model: BalanceSource) -> some View {
        MonacoErrorRow(thing: "your balance", identifier: "\(identifierPrefix)-balance-error") {
            Task { await model.load() }
        }
    }

    @ViewBuilder
    private var actions: some View {
        if dynamicTypeSize.isAccessibilitySize {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                link("Add money", DepositRoute(), id: "\(identifierPrefix)-add-money-link")
                link("Withdraw", WithdrawRoute(), id: "\(identifierPrefix)-withdraw-link")
            }
        } else {
            HStack(spacing: MonacoTheme.Space.sm) {
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
                .padding(.horizontal, MonacoTheme.Space.sm)
                .padding(.vertical, MonacoTheme.Space.s)
                .background(MonacoTheme.brandWash, in: Capsule())
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
