import MonacoAPI
import MonacoCore
import SwiftUI

enum ProfileMoneyValue: Equatable {
    case loading
    case amount(Int64)
    case unavailable
}

enum ProfileCashSlot: ProfileSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        ProfileCashSection()
    }
}

enum ProfileCashMetrics {
    static let skeletonWidth: CGFloat = 96
    static let glyphWidth: CGFloat = 40
    static let infoGlyph: CGFloat = 17
    static let infoTarget: CGFloat = 44
}

struct ProfileCashRow: View {
    let cash: ProfileMoneyValue
    var status: String?
    var arrivingMicros: Int64 = 0

    @State private var showsInfo = false

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
            MonacoCard(style: .tight) { row }
                .background(alignment: .center) { link }
                .accessibilityElement(children: .contain)
            arriving
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.l - SectionStackMetrics.spacing)
        .frame(maxWidth: .infinity, alignment: .leading)
        .sheet(isPresented: $showsInfo) { ProfileApyInfoSheet() }
    }

    private var link: some View {
        NavigationLink(value: AnyAppRoute(BalancesRoute())) {
            Rectangle().fill(Color.clear).contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityLabel(ProfileCopy.cashLabel)
        .accessibilityHint("Opens your balances")
        .accessibilityIdentifier("profile-cash-row")
    }

    private var row: some View {
        HStack(spacing: MonacoTheme.Space.sm) {
            Image(systemName: MonacoIcon.card.symbol)
                .font(MonacoTheme.Typo.title2)
                .foregroundStyle(MonacoTheme.primaryText)
                .frame(width: ProfileCashMetrics.glyphWidth)
                .accessibilityHidden(true)
                .allowsHitTesting(false)
            VStack(alignment: .leading, spacing: 0) {
                Text(ProfileCopy.cashLabel)
                    .font(MonacoTheme.Typo.headline)
                    .foregroundStyle(MonacoTheme.primaryText)
                    .allowsHitTesting(false)
                    .accessibilityHidden(true)
                HStack(spacing: 0) {
                    Text(ProfileCopy.apyLabel)
                        .font(MonacoTheme.Typo.subhead)
                        .foregroundStyle(MonacoTheme.secondaryText)
                        .allowsHitTesting(false)
                    infoButton
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            figure
                .lineLimit(1)
                .minimumScaleFactor(MoneyStyle.row.minimumScaleFactor)
                .allowsHitTesting(false)
                .accessibilityElement(children: .ignore)
                .accessibilityLabel("Cash")
                .accessibilityValue(Self.spoken(cash))
                .accessibilityIdentifier("profile-balance-value")
        }
        .frame(minHeight: MonacoButtonMetrics.hitTarget)
    }

    private var infoButton: some View {
        Button {
            showsInfo = true
        } label: {
            Image(systemName: MonacoIcon.info.symbol)
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.tint)
                .frame(width: ProfileCashMetrics.infoTarget, height: ProfileCashMetrics.infoTarget)
                .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .padding(.vertical, -(ProfileCashMetrics.infoTarget - ProfileCashMetrics.infoGlyph) / 2)
        .padding(.leading, MonacoTheme.Space.xs)
        .accessibilityLabel("About \(ProfileCopy.apyLabel)")
        .accessibilityIdentifier("profile-apy-info")
    }

    @ViewBuilder private var figure: some View {
        switch cash {
        case .loading:
            Text("$0.00").moneyFont(.row).skeletonBar(width: ProfileCashMetrics.skeletonWidth)
        case .amount(let micros):
            MoneyText(micros: UsdAmountFormatter.flooredToCents(micros), style: .row)
        case .unavailable:
            Text("\u{2014}").moneyFont(.row).foregroundStyle(MonacoTheme.muted)
        }
    }

    static func spoken(_ value: ProfileMoneyValue) -> String {
        switch value {
        case .loading: "loading"
        case .amount(let micros): UsdAmountFormatter.format(micros: UsdAmountFormatter.flooredToCents(micros))
        case .unavailable: "unavailable"
        }
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

private struct ProfileApyInfoSheet: View {
    var body: some View {
        Text(ProfileCopy.apyInfo)
            .font(MonacoTheme.Typo.subhead)
            .foregroundStyle(MonacoTheme.secondaryText)
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.bottom, MonacoTheme.Space.m)
            .frame(maxWidth: .infinity, alignment: .leading)
            .monacoFittedSheet(title: ProfileCopy.apyInfoTitle, showsClose: true)
            .accessibilityIdentifier("profile-apy-info-sheet")
    }
}

struct ProfileCashSection: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(ScreenRefresh.self) private var refresh: ScreenRefresh?
    @Environment(\.scenePhase) private var scenePhase

    var body: some View {
        let state = environment.balance.state
        ProfileCashRow(
            cash: Self.cash(state),
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

    private static func inFlight(_ state: LoadState<AccountBalance>) -> Int64 {
        if case .loaded(let balance) = state { return balance.inFlightMicros }
        return 0
    }

    private static func arriving(_ state: LoadState<AccountBalance>) -> Int64 {
        if case .loaded(let balance) = state { return balance.arrivingMicros }
        return 0
    }
}
