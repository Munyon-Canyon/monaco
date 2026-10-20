import MonacoAPI
import MonacoCore
import SwiftUI

enum ProfileMoneyValue: Equatable {
    case loading
    case amount(Int64)
    case unavailable
}

enum ProfileMoneyStripMetrics {
    static let netWorthWeight: CGFloat = 1.4
    static let skeletonWidth: CGFloat = 96
    static let caption = "Account balances are visible only to you."
}

struct ProfileMoneyStrip: View {
    let netWorth: ProfileMoneyValue
    let cash: ProfileMoneyValue
    var status: String?
    var arrivingMicros: Int64 = 0

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @Environment(\.depositNavigator) private var depositNavigator
    @Environment(\.displayScale) private var displayScale

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
            VStack(spacing: 0) {
                MonacoRule(color: MonacoTheme.hairline)
                strip.padding(.top, MonacoTheme.Space.m)
            }
            Text(ProfileMoneyStripMetrics.caption)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
                .accessibilityIdentifier("profile-balance-caption")
            arriving
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    @ViewBuilder private var strip: some View {
        if dynamicTypeSize.isAccessibilitySize {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
                HStack(alignment: .top, spacing: MonacoTheme.Space.s) {
                    netWorthMetric
                    cashMetric
                }
                deposit(fillsWidth: true)
            }
        } else {
            ProfileMoneyStripRow(spacing: MonacoTheme.Space.s, weights: [ProfileMoneyStripMetrics.netWorthWeight, 1]) {
                netWorthMetric
                    .padding(.trailing, MonacoTheme.Space.s)
                    .overlay(alignment: .trailing) {
                        Rectangle().fill(MonacoTheme.hairline).frame(width: 1 / displayScale)
                    }
                cashMetric
                deposit(fillsWidth: false)
            }
        }
    }

    private var netWorthMetric: some View {
        metric("Net worth", value: netWorth, identifier: "profile-net-worth")
    }

    private var cashMetric: some View {
        metric("Cash", value: cash, identifier: "profile-balance-value")
    }

    private func metric(_ label: String, value: ProfileMoneyValue, identifier: String) -> some View {
        NavigationLink(value: AnyAppRoute(BalancesRoute())) {
            VStack(alignment: .leading, spacing: 0) {
                Text(label)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
                figure(value)
                    .lineLimit(1)
                    .minimumScaleFactor(MoneyStyle.row.minimumScaleFactor)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .frame(minHeight: MonacoButtonMetrics.hitTarget)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(label)
        .accessibilityValue(Self.spoken(value))
        .accessibilityIdentifier(identifier)
    }

    @ViewBuilder private func figure(_ value: ProfileMoneyValue) -> some View {
        switch value {
        case .loading:
            Text("$0.00").moneyFont(.row).skeletonBar(width: ProfileMoneyStripMetrics.skeletonWidth)
        case .amount(let micros):
            MoneyText(micros: UsdAmountFormatter.flooredToCents(micros), style: .row)
        case .unavailable:
            Text("—").moneyFont(.row).foregroundStyle(MonacoTheme.muted)
        }
    }

    static func spoken(_ value: ProfileMoneyValue) -> String {
        switch value {
        case .loading: "loading"
        case .amount(let micros): UsdAmountFormatter.format(micros: UsdAmountFormatter.flooredToCents(micros))
        case .unavailable: "unavailable"
        }
    }

    private func deposit(fillsWidth: Bool) -> some View {
        Button {
            depositNavigator?.presentDeposit()
        } label: {
            Text("Deposit").frame(maxWidth: fillsWidth ? .infinity : nil)
        }
        .buttonStyle(.monacoCompactProminent)
        .accessibilityIdentifier("profile-add-money-link")
    }

    @ViewBuilder private var arriving: some View {
        if let status {
            Text(status)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.warning)
                .accessibilityIdentifier("platform-balance-status")
        }
        if arrivingMicros > 0 {
            HStack(spacing: MonacoTheme.Space.xs) {
                Text(PlatformBalanceCard.arrivingLabel)
                    .font(MonacoTheme.Typo.caption)
                MoneyText(
                    micros: UsdAmountFormatter.flooredToCents(arrivingMicros), style: .caption,
                    color: MonacoTheme.warning
                )
                .accessibilityIdentifier("platform-balance-arriving-value")
            }
            .foregroundStyle(MonacoTheme.warning)
            .accessibilityElement(children: .combine)
            .accessibilityIdentifier("platform-balance-arriving")
        }
    }
}

private struct ProfileMoneyStripRow: Layout {
    let spacing: CGFloat
    let weights: [CGFloat]

    private func widths(_ subviews: Subviews, in total: CGFloat) -> [CGFloat] {
        let button = subviews[2].sizeThatFits(.unspecified).width
        let room = max(total - button - spacing * 2, 0)
        let shares = weights.reduce(0, +)
        return weights.map { room * $0 / shares } + [button]
    }

    func sizeThatFits(proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) -> CGSize {
        guard subviews.count == 3 else { return .zero }
        let width = proposal.width ?? 0
        let heights = zip(subviews, widths(subviews, in: width)).map {
            $0.sizeThatFits(ProposedViewSize(width: $1, height: nil)).height
        }
        return CGSize(width: width, height: heights.max() ?? 0)
    }

    func placeSubviews(in bounds: CGRect, proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) {
        guard subviews.count == 3 else { return }
        var x = bounds.minX
        for (subview, width) in zip(subviews, widths(subviews, in: bounds.width)) {
            subview.place(
                at: CGPoint(x: x, y: bounds.midY), anchor: .leading,
                proposal: ProposedViewSize(width: width, height: nil))
            x += width + spacing
        }
    }
}

struct ProfileMoneyStripSection: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(ScreenRefresh.self) private var refresh: ScreenRefresh?
    @Environment(PortfolioModel.self) private var portfolio: PortfolioModel?
    @Environment(\.scenePhase) private var scenePhase

    var body: some View {
        let state = environment.balance.state
        let cash = Self.cash(state)
        ProfileMoneyStrip(
            netWorth: Self.netWorth(cash: cash, portfolio: portfolio?.state ?? .loading),
            cash: cash,
            status: PlatformBalanceCard.statusLine(
                cardProcessing: environment.cardDeposit.isProcessing, pendingMicros: Self.inFlight(state)),
            arrivingMicros: Self.arriving(state)
        )
        .task {
            refresh?.register("profile-balance") { await environment.balance.load() }
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

    static func cash(_ state: LoadState<AccountBalance>) -> ProfileMoneyValue {
        switch state {
        case .idle, .loading: .loading
        case .loaded(let balance): .amount(balance.availableMicros)
        case .failed: .unavailable
        }
    }

    static func netWorth(cash: ProfileMoneyValue, portfolio: LoadState<PortfolioSummary>) -> ProfileMoneyValue {
        switch (cash, portfolio) {
        case (.amount(let micros), .loaded(let summary)): .amount(summary.netWorthMicros(cashMicros: micros))
        case (.unavailable, _), (_, .failed): .unavailable
        default: .loading
        }
    }

    private static func inFlight(_ state: LoadState<AccountBalance>) -> Int64 {
        if case .loaded(let balance) = state { return balance.inFlightMicros }
        return 0
    }

    private static func arriving(_ state: LoadState<AccountBalance>) -> Int64 {
        if case .loaded(let balance) = state { return balance.arrivingMicros }
        return 0
    }
}
