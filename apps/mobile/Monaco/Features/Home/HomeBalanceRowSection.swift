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
    @Environment(ScreenRefresh.self) private var refresh: ScreenRefresh?
    @Environment(\.homeReads) private var reads
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @Environment(\.scenePhase) private var scenePhase

    static func showsRetryRow(_ state: LoadState<AccountBalance>) -> Bool {
        if case .failed = state { return true }
        return false
    }

    var body: some View {
        MonacoGroupedList {
            if Self.showsRetryRow(environment.balance.state), reads.showsOwnRow(.balance) {
                retryRow
            } else {
                PlatformBalanceCard(
                    state: environment.balance.state, cardProcessing: environment.cardDeposit.isProcessing,
                    valueIdentifier: balanceIdentifier)
            }
            actions
                .padding(.leading, PlatformBalanceCard.leadingInset)
                .padding(.trailing, MonacoTheme.Space.gutter)
        }
        .onChange(of: HomeReadStatus(environment.balance.state), initial: true) { _, status in
            reads?.report(.balance, status)
        }
        .task {
            refresh?.register("balance") { await environment.balance.load() }
            await environment.balance.load()
        }
        .task {
            await environment.cardDeposit.observe()
        }
        .onChange(of: scenePhase) { _, phase in
            guard phase == .active else { return }
            Task { await environment.cardDeposit.foregrounded() }
        }
    }

    private var retryRow: some View {
        MonacoErrorRow(thing: "your balance", identifier: "\(identifierPrefix)-balance-error") {
            Task { await environment.balance.load() }
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
}
