import MonacoAPI
import MonacoCore
import SwiftUI

enum HomeCashRowMetrics {
    static let skeletonWidth: CGFloat = 96
    static let tile: CGFloat = 32
    static let infoTarget: CGFloat = 44
    static let partColumn: CGFloat = 56
}

struct HomeCashRow: View {
    let display: HomeBalanceDisplay
    var status: String?
    var arrivingMicros: Int64 = 0
    var embedded = false
    var retry: (() -> Void)?

    @Environment(\.depositNavigator) private var depositNavigator
    @Environment(\.homeCashEmphasis) private var emphasis
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @State private var showsInfo = false
    @ScaledMetric(relativeTo: .headline) private var tileSize: CGFloat = HomeCashRowMetrics.tile

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            if embedded {
                row
                    .accessibilityElement(children: .contain)
                    .accessibilityIdentifier("home-cash-row")
            } else {
                MonacoCard(style: .tight) { row }
                    .accessibilityElement(children: .contain)
                    .accessibilityIdentifier("home-cash-row")
            }
            if let retry {
                MonacoErrorRow(
                    thing: "your balance", identifier: "home-balance-error", inset: false, retry: retry)
            }
            arriving
        }
        .padding(.horizontal, embedded ? 0 : MonacoTheme.Space.gutter)
        .frame(maxWidth: .infinity, alignment: .leading)
        .sheet(isPresented: $showsInfo) { HomeApyInfoSheet() }
    }

    @ViewBuilder private var row: some View {
        if dynamicTypeSize.isAccessibilitySize {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
                summary
                deposit.frame(maxWidth: .infinity)
            }
        } else {
            HStack(spacing: MonacoTheme.Space.sm) {
                summary
                deposit
            }
        }
    }

    @ViewBuilder private var summary: some View {
        if embedded {
            partSummary
        } else if dynamicTypeSize.isAccessibilitySize {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                HStack(alignment: .top, spacing: MonacoTheme.Space.sm) {
                    cardTile
                    labels
                }
                cashFigure
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        } else {
            inlineSummary
        }
    }

    private var partSummary: some View {
        HStack(spacing: MonacoTheme.Space.sm) {
            cardTile.frame(width: max(HomeCashRowMetrics.partColumn, tileSize))
            VStack(alignment: .leading, spacing: 0) {
                figure
                    .lineLimit(1)
                    .minimumScaleFactor(MoneyStyle.row.minimumScaleFactor)
                Text(HomeFigureCopy.cashLine)
                    .font(MonacoTheme.Typo.subhead)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .accessibilityIdentifier("home-cash-label")
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .frame(minHeight: MonacoButtonMetrics.hitTarget)
            .contentShape(.accessibility, Rectangle())
            .accessibilityElement(children: .ignore)
            .accessibilityLabel("Cash")
            .accessibilityValue(spoken)
            .accessibilityIdentifier(valueIdentifier)
        }
    }

    private var cardTile: some View {
        Image(systemName: MonacoIcon.card.symbol)
            .font(MonacoTheme.Typo.section)
            .foregroundStyle(MonacoTheme.primaryText)
            .frame(width: tileSize, height: tileSize)
            .accessibilityHidden(true)
    }

    private var labels: some View {
        VStack(alignment: .leading, spacing: 0) {
            Text(MoneyCopy.cashLabel)
                .font(MonacoTheme.Typo.headline)
                .foregroundStyle(MonacoTheme.primaryText)
                .accessibilityIdentifier("home-cash-label")
            if LaunchFeatures.apy {
                HStack(alignment: .top, spacing: 0) {
                    Text(ProfileCopy.apyLabel)
                        .font(MonacoTheme.Typo.subhead)
                        .foregroundStyle(MonacoTheme.secondaryText)
                        .fixedSize(horizontal: false, vertical: true)
                    infoButton
                }
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private var cashFigure: some View {
        figure
            .lineLimit(1)
            .minimumScaleFactor(MoneyStyle.row.minimumScaleFactor)
            .frame(minHeight: MonacoButtonMetrics.hitTarget)
            .contentShape(.accessibility, Rectangle())
            .accessibilityElement(children: .ignore)
            .accessibilityLabel("Cash")
            .accessibilityValue(spoken)
            .accessibilityIdentifier(valueIdentifier)
    }

    private var inlineSummary: some View {
        HStack(spacing: MonacoTheme.Space.sm) {
            cardTile
            labels
            cashFigure
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
                .frame(width: HomeCashRowMetrics.infoTarget, height: HomeCashRowMetrics.infoTarget)
                .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .padding(.vertical, -HomeCashRowMetrics.infoTarget / 4)
        .padding(.leading, MonacoTheme.Space.xs)
        .accessibilityLabel("About \(ProfileCopy.apyLabel)")
        .accessibilityIdentifier("home-apy-info")
    }

    @ViewBuilder private var figure: some View {
        switch display {
        case .loading:
            Text("$0.00").moneyFont(.row).skeletonBar(width: HomeCashRowMetrics.skeletonWidth)
        case .amount(let micros):
            MoneyText(micros: UsdAmountFormatter.flooredToCents(micros), style: .row)
        case .unavailable:
            Text("\u{2014}").moneyFont(.row).foregroundStyle(MonacoTheme.muted)
        }
    }

    private var valueIdentifier: String {
        switch display {
        case .loading: "platform-balance-loading"
        case .amount: "platform-balance-value"
        case .unavailable: "platform-balance-unavailable"
        }
    }

    private var spoken: String {
        switch display {
        case .loading: "loading"
        case .amount(let micros): UsdAmountFormatter.format(micros: UsdAmountFormatter.flooredToCents(micros))
        case .unavailable: "unavailable"
        }
    }

    @ViewBuilder private var deposit: some View {
        let button = Button {
            depositNavigator?.presentDeposit()
        } label: {
            Text("Deposit")
        }
        .accessibilityIdentifier("home-add-money-link")
        switch emphasis {
        case .prominent: button.buttonStyle(.monacoCompactProminent)
        case .quiet: button.buttonStyle(.monacoCompact)
        }
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

private struct HomeApyInfoSheet: View {
    var body: some View {
        Text(ProfileCopy.apyInfo)
            .font(MonacoTheme.Typo.subhead)
            .foregroundStyle(MonacoTheme.secondaryText)
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.bottom, MonacoTheme.Space.m)
            .frame(maxWidth: .infinity, alignment: .leading)
            .monacoFittedSheet(title: ProfileCopy.apyInfoTitle, showsClose: true)
            .accessibilityIdentifier("home-apy-info-sheet")
    }
}

struct HomeCashRowSection: View {
    var embedded = false

    @Environment(AppEnvironment.self) private var environment
    @Environment(ScreenRefresh.self) private var refresh: ScreenRefresh?
    @Environment(PortfolioModel.self) private var portfolio: PortfolioModel?
    @Environment(\.homeReads) private var reads
    @Environment(\.scenePhase) private var scenePhase

    var body: some View {
        let state = environment.balance.state
        let gate = HomeGate(portfolio: portfolio, reads: reads, balance: state)
        let display = HomeBalanceDisplay.resolve(state)
        HomeCashRow(
            display: display,
            status: PlatformBalanceCard.statusLine(
                cardProcessing: environment.cardDeposit.isProcessing, pendingMicros: inFlight(state)),
            arrivingMicros: arriving(state),
            embedded: embedded,
            retry: HomeBalanceRowSection.showsRetryRow(state) && reads.showsOwnRow(.balance)
                ? { Task { await environment.balance.load() } } : nil
        )
        .homeSlotHidden(!embedded && (!gate.shows { $0.showsMoneyStrip } || partsCardShows))
        .onChange(of: HomeReadStatus(state), initial: true) { _, status in
            reads?.report(.balance, status)
        }
        .task(id: ownsReads) {
            guard ownsReads else { return }
            refresh?.register("balance") { await environment.balance.load() }
            await environment.balance.loadIfNeeded()
        }
        .task(id: ownsReads) {
            guard ownsReads else { return }
            await environment.cardDeposit.observe()
        }
        .onChange(of: scenePhase) { _, phase in
            guard phase == .active, ownsReads else { return }
            Task { await environment.cardDeposit.foregrounded() }
        }
    }

    private var partsCardShows: Bool {
        HomeTotalPartsSlot.shows(
            summary: portfolio?.summary, sharedFailure: reads?.plan.showsSharedRow == true)
    }

    private var ownsReads: Bool { embedded == partsCardShows }

    private func inFlight(_ state: LoadState<AccountBalance>) -> Int64 {
        if case .loaded(let balance) = state { return balance.inFlightMicros }
        return 0
    }

    private func arriving(_ state: LoadState<AccountBalance>) -> Int64 {
        if case .loaded(let balance) = state { return balance.arrivingMicros }
        return 0
    }
}
