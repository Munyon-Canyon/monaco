import MonacoAPI
import MonacoCore
import SwiftUI

enum HomeMoneyStripMetrics {
    static let netWorthWeight: CGFloat = 1.4
    static let accessibilityRuleHeight: CGFloat = 1
}

struct HomeMoneyStrip: View {
    let netWorthMicros: Int64?
    let display: HomeBalanceDisplay
    var status: String?
    var arrivingMicros: Int64 = 0
    var retry: (() -> Void)?

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @Environment(\.depositNavigator) private var depositNavigator
    @Environment(\.displayScale) private var displayScale

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            VStack(spacing: 0) {
                MonacoRule(color: MonacoTheme.hairline)
                Text("Your money")
                    .font(MonacoTheme.Typo.subheadStrong)
                    .foregroundStyle(MonacoTheme.ink)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(.top, MonacoTheme.Space.m)
                    .accessibilityAddTraits(.isHeader)
                    .accessibilityIdentifier("home-money-title")
                strip.padding(.top, MonacoTheme.Space.xs)
            }
            if let retry {
                MonacoErrorRow(
                    thing: "your balance", identifier: "home-balance-error", inset: false, retry: retry)
            }
            arriving
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    @ViewBuilder private var strip: some View {
        if dynamicTypeSize.isAccessibilitySize {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
                netWorth
                MonacoRule(color: MonacoTheme.hairline)
                cash
                MonacoRule(color: MonacoTheme.hairline)
                deposit(fillsWidth: true)
            }
        } else {
            HomeMoneyStripRow(spacing: MonacoTheme.Space.s, weights: [HomeMoneyStripMetrics.netWorthWeight, 1]) {
                netWorth
                    .padding(.trailing, MonacoTheme.Space.s)
                    .overlay(alignment: .trailing) {
                        Rectangle().fill(MonacoTheme.hairline).frame(width: 1 / displayScale)
                    }
                cash
                deposit(fillsWidth: false)
            }
        }
    }

    private var netWorth: some View {
        let text = netWorthMicros.map { Self.spoken($0) }
        return metric("Net worth", spoken: text ?? Self.unavailableText, identifier: "home-net-worth") {
            switch display {
            case .loading: placeholderBar
            case .amount, .unavailable:
                if let netWorthMicros {
                    MoneyText(micros: UsdAmountFormatter.flooredToCents(netWorthMicros), style: .row)
                } else {
                    unavailable
                }
            }
        }
    }

    private var cash: some View {
        switch display {
        case .loading:
            return AnyView(
                metric("Cash", spoken: Self.loadingText, identifier: "platform-balance-loading") { placeholderBar })
        case .amount(let micros):
            return AnyView(
                metric("Cash", spoken: Self.spoken(micros), identifier: "platform-balance-value") {
                    MoneyText(micros: UsdAmountFormatter.flooredToCents(micros), style: .row)
                })
        case .unavailable:
            return AnyView(
                metric("Cash", spoken: Self.unavailableText, identifier: "platform-balance-unavailable") {
                    unavailable
                })
        }
    }

    private static let unavailableText = "unavailable"
    private static let loadingText = "loading"

    private static func spoken(_ micros: Int64) -> String {
        UsdAmountFormatter.format(micros: UsdAmountFormatter.flooredToCents(micros))
    }

    private var placeholderBar: some View {
        Text("$0.00").moneyFont(.row).skeletonBar(width: 96)
    }

    private var unavailable: some View {
        Text("—")
            .moneyFont(.row)
            .foregroundStyle(MonacoTheme.muted)
    }

    private func metric<Value: View>(
        _ label: String, spoken: String, identifier: String, @ViewBuilder value: () -> Value
    ) -> some View {
        NavigationLink(value: AnyAppRoute(BalancesRoute())) {
            VStack(alignment: .leading, spacing: 0) {
                Text(label)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
                value()
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
        .accessibilityValue(spoken)
        .accessibilityIdentifier(identifier)
    }

    private func deposit(fillsWidth: Bool) -> some View {
        Button {
            depositNavigator?.presentDeposit()
        } label: {
            Text("Deposit").frame(maxWidth: fillsWidth ? .infinity : nil)
        }
        .buttonStyle(.monacoCompactProminent)
        .accessibilityIdentifier("home-add-money-link")
    }

    @ViewBuilder private var arriving: some View {
        if let status {
            Text(status)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.warning)
                .accessibilityIdentifier("platform-balance-status")
        }
        if PlatformBalanceCard.showsArriving(display: display, micros: arrivingMicros) {
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

private struct HomeMoneyStripRow: Layout {
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

struct HomeMoneyStripSection: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(ScreenRefresh.self) private var refresh: ScreenRefresh?
    @Environment(PortfolioModel.self) private var portfolio: PortfolioModel?
    @Environment(\.homeReads) private var reads
    @Environment(\.scenePhase) private var scenePhase

    var body: some View {
        let state = environment.balance.state
        let gate = HomeGate(portfolio: portfolio, reads: reads, balance: state)
        let display = HomeBalanceDisplay.resolve(state)
        HomeMoneyStrip(
            netWorthMicros: netWorth(gate: gate, display: display),
            display: display,
            status: PlatformBalanceCard.statusLine(
                cardProcessing: environment.cardDeposit.isProcessing, pendingMicros: inFlight(state)),
            arrivingMicros: arriving(state),
            retry: HomeBalanceRowSection.showsRetryRow(state) && reads.showsOwnRow(.balance)
                ? { Task { await environment.balance.load() } } : nil
        )
        .padding(.top, MonacoTheme.Space.m - MonacoTheme.Space.section)
        .homeSlotHidden(!gate.shows { $0.showsMoneyStrip })
        .onChange(of: HomeReadStatus(state), initial: true) { _, status in
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

    private func netWorth(gate: HomeGate, display: HomeBalanceDisplay) -> Int64? {
        guard let summary = gate.summary, case .amount = display else { return nil }
        return summary.netWorthMicros(cashMicros: gate.cashMicros)
    }

    private func inFlight(_ state: LoadState<AccountBalance>) -> Int64 {
        if case .loaded(let balance) = state { return balance.inFlightMicros }
        return 0
    }

    private func arriving(_ state: LoadState<AccountBalance>) -> Int64 {
        if case .loaded(let balance) = state { return balance.arrivingMicros }
        return 0
    }
}
