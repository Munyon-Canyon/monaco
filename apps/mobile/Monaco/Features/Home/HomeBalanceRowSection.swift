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
    @Environment(\.scenePhase) private var scenePhase

    static func showsRetryRow(_ state: LoadState<AccountBalance>) -> Bool {
        if case .failed = state { return true }
        return false
    }

    var body: some View {
        let state = environment.balance.state
        HomeBalanceSummary(
            state: state, cardProcessing: environment.cardDeposit.isProcessing,
            identifierPrefix: identifierPrefix, valueIdentifier: balanceIdentifier,
            retry: Self.showsRetryRow(state) && reads.showsOwnRow(.balance)
                ? { Task { await environment.balance.load() } } : nil
        )
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
}

struct HomeBalanceSummary: View {
    let display: HomeBalanceDisplay
    var status: String?
    var arrivingMicros: Int64 = 0
    var identifierPrefix = "home"
    var valueIdentifier = "platform-balance-value"
    var retry: (() -> Void)?

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @Environment(\.depositNavigator) private var depositNavigator

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            if let retry {
                MonacoErrorRow(
                    thing: "your balance", identifier: "\(identifierPrefix)-balance-error", inset: false, retry: retry)
            } else {
                figure
            }
            actions
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.vertical, MonacoTheme.Space.m)
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private var figure: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
            Text("Account balance")
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.secondaryText)
            amount
            if let status {
                Text(status)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .accessibilityIdentifier("platform-balance-status")
            }
            if PlatformBalanceCard.showsArriving(display: display, micros: arrivingMicros) {
                HStack(spacing: MonacoTheme.Space.xs) {
                    Text(PlatformBalanceCard.arrivingLabel)
                        .font(MonacoTheme.Typo.caption)
                    MoneyText(
                        micros: UsdAmountFormatter.flooredToCents(arrivingMicros), style: .caption,
                        color: MonacoTheme.secondaryText
                    )
                    .accessibilityIdentifier("platform-balance-arriving-value")
                }
                .foregroundStyle(MonacoTheme.secondaryText)
                .accessibilityElement(children: .combine)
                .accessibilityIdentifier("platform-balance-arriving")
            }
        }
    }

    @ViewBuilder
    private var amount: some View {
        switch display {
        case .loading:
            SkeletonBlock(width: 140, height: 32)
                .accessibilityElement()
                .accessibilityLabel("Loading your account balance")
                .accessibilityIdentifier("platform-balance-loading")
        case .amount(let micros):
            MoneyText(micros: UsdAmountFormatter.flooredToCents(micros), style: .large)
                .accessibilityIdentifier(valueIdentifier)
        case .unavailable:
            Text("—")
                .moneyFont(.large)
                .foregroundStyle(MonacoTheme.secondaryText)
                .accessibilityLabel("Account balance unavailable")
                .accessibilityIdentifier("platform-balance-unavailable")
        }
    }

    private var actions: some View {
        let layout =
            dynamicTypeSize.isAccessibilitySize
            ? AnyLayout(VStackLayout(spacing: MonacoTheme.Space.s))
            : AnyLayout(HStackLayout(spacing: MonacoTheme.Space.sm))
        return layout {
            Button {
                depositNavigator?.presentDeposit()
            } label: {
                Text("Add money").frame(maxWidth: .infinity)
            }
            .buttonStyle(.monacoCompact)
            .accessibilityIdentifier("\(identifierPrefix)-add-money-link")
            link("Withdraw", WithdrawRoute(), id: "\(identifierPrefix)-withdraw-link")
        }
    }

    private func link(_ title: String, _ route: some AppRoute, id: String) -> some View {
        NavigationLink(value: AnyAppRoute(route)) {
            Text(title).frame(maxWidth: .infinity)
        }
        .buttonStyle(.monacoCompact)
        .accessibilityIdentifier(id)
    }
}

extension HomeBalanceSummary {
    init(
        state: LoadState<AccountBalance>, cardProcessing: Bool, identifierPrefix: String, valueIdentifier: String,
        retry: (() -> Void)?
    ) {
        var inFlightMicros: Int64 = 0
        var arrivingMicros: Int64 = 0
        if case .loaded(let balance) = state {
            inFlightMicros = balance.inFlightMicros
            arrivingMicros = balance.arrivingMicros
        }
        self.init(
            display: .resolve(state),
            status: PlatformBalanceCard.statusLine(cardProcessing: cardProcessing, pendingMicros: inFlightMicros),
            arrivingMicros: arrivingMicros, identifierPrefix: identifierPrefix, valueIdentifier: valueIdentifier,
            retry: retry)
    }
}
