import MonacoAPI
import MonacoCore
import SwiftUI

enum CabalTreasuryTabSlot: CabalSection {
    static let isLive = true

    static func body(for context: CabalContext) -> some View {
        CabalTreasuryTab(cabalID: context.cabalID)
    }
}

enum CabalTreasuryCopy {
    static let historyHint = "Opens Treasury history"
    static let loading = "Loading the treasury"
    static let failedThing = "the treasury"
    static let botDetail = "Opens the propose chooser"

    static func zeroPotNote(canPropose: Bool) -> String {
        canPropose ? "Fund, then propose the first buy." : "Nothing in the pot yet."
    }

    static func cashOnlyNote(canPropose: Bool) -> String {
        canPropose ? "No stocks right now. Propose a buy to put the cash to work." : "No stocks right now."
    }
}

enum CabalTreasuryMetrics {
    static let sectionGap = MonacoTheme.Space.l
}

struct CabalTreasuryTab: View {
    let cabalID: String

    @Environment(AppEnvironment.self) private var environment
    @Environment(\.hostMainTab) private var hostMainTab

    var body: some View {
        CabalPotModelHost(cabalID: cabalID, showsLogos: true) { model in
            CabalMoveReader(cabalID: cabalID) { move in
                CabalTreasuryBody(model: model, cabalID: cabalID, move: move) { route in
                    environment.navigator.open(route, in: hostMainTab ?? environment.navigator.selectedTab)
                }
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("cabal-treasury")
    }
}

struct CabalTreasuryBody: View {
    let model: CabalPotModel?
    let cabalID: String
    let move: CabalHeaderCopy.Move?
    let open: (any AppRoute) -> Void

    @Environment(ToastCenter.self) private var toasts
    @Environment(\.cabalModel) private var cabalModel

    private var canPropose: Bool { cabalModel?.cabal?.me?.canVote == true }

    var body: some View {
        content
            .frame(maxWidth: .infinity, alignment: .leading)
            .onChange(of: model?.toast) { _, message in
                guard let model, let message else { return }
                toasts.current = MonacoToast(message: message)
                model.dismissToast()
            }
    }

    static func showsBotRow(isLive: Bool = ProposeChooserConfig.isBotProposalLive) -> Bool { isLive }

    @ViewBuilder private var content: some View {
        switch model?.state ?? .loading {
        case .idle, .loading:
            MonacoRowSkeleton(rows: 3, markShape: .tile, hasTrailing: true)
                .accessibilityElement()
                .accessibilityLabel(CabalTreasuryCopy.loading)
                .accessibilityIdentifier("cabal-holdings-loading")
        case .failed:
            MonacoErrorRow(thing: CabalTreasuryCopy.failedThing, identifier: "cabal-holdings-failed") {
                Task { await model?.load() }
            }
        case .loaded(let summary):
            loaded(summary)
        }
    }

    @ViewBuilder
    private func loaded(_ summary: CabalPotSummary) -> some View {
        if summary.state == .zero {
            VStack(alignment: .leading, spacing: CabalTreasuryMetrics.sectionGap) {
                note(CabalTreasuryCopy.zeroPotNote(canPropose: canPropose), id: "cabal-holdings-empty")
                CabalTreasuryActions(
                    cabalID: cabalID, cashOut: nil, history: CabalTreasuryRows.history(move), open: open)
            }
        } else {
            let rows = summary.treasury
            VStack(alignment: .leading, spacing: CabalTreasuryMetrics.sectionGap) {
                CabalTreasuryCashCard(cash: rows.cash)
                if rows.positions.isEmpty {
                    note(CabalTreasuryCopy.cashOnlyNote(canPropose: canPropose), id: "cabal-holdings-nothing-bought")
                } else {
                    positions(rows.positions)
                }
                CabalTreasuryActions(
                    cabalID: cabalID, cashOut: rows.cashOut, history: CabalTreasuryRows.history(move), open: open)
            }
        }
    }

    private func positions(_ positions: [CabalTreasuryRows.Position]) -> some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            MonacoSectionHeader(CabalTreasuryRows.positionsLabel, count: positions.count)
                .secondaryCount()
                .padding(.horizontal, MonacoTheme.Space.gutter)
            MonacoGroupedList {
                ForEach(positions) { position in
                    Button {
                        Haptics.selection()
                        open(CabalHoldingRoute(cabalID: cabalID, symbol: position.symbol))
                    } label: {
                        CabalTreasuryPositionRow(position: position, isLast: position.id == positions.last?.id)
                    }
                    .buttonStyle(.monacoRow)
                    .accessibilityIdentifier("cabal-holding-\(position.ticker)")
                }
            }
        }
    }

    private func note(_ text: String, id: String) -> some View {
        Text(text)
            .font(MonacoTheme.Typo.subhead)
            .foregroundStyle(MonacoTheme.secondaryText)
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .accessibilityIdentifier(id)
    }
}

struct CabalTreasuryPositionRow: View {
    let position: CabalTreasuryRows.Position
    let isLast: Bool

    var body: some View {
        MarketStockRow(
            name: position.name, subtitle: position.detail,
            mark: StockMark(symbol: position.symbol, displayName: position.name, logoURL: position.logoURL),
            isLast: isLast
        ) {
            Text(position.value)
                .moneyFigure(.row)
                .foregroundStyle(MonacoTheme.ink)
                .lineLimit(1)
            trailing
        }
    }

    @ViewBuilder private var trailing: some View {
        switch position.trailing {
        case .change(let text, let direction):
            Text(text)
                .moneyFigure(.caption)
                .foregroundStyle(Self.color(direction))
                .lineLimit(1)
                .accessibilityLabel(PnLSpeech.percent(text))
        case .estimate(let text):
            Text(text)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
                .lineLimit(1)
        case .none:
            EmptyView()
        }
    }

    private static func color(_ direction: PortfolioSummary.Direction) -> Color {
        switch direction {
        case .up: MonacoTheme.profit
        case .down: MonacoTheme.loss
        case .flat: MonacoTheme.secondaryText
        }
    }
}

struct CabalTreasuryCashCard: View {
    let cash: CabalTreasuryRows.Cash

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @State private var showsInfo = false

    var body: some View {
        MonacoCard(style: .tight) {
            ViewThatFits(in: .horizontal) {
                HStack(spacing: MonacoTheme.Space.sm) {
                    tile
                    labels
                    amount
                }
                VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                    HStack(spacing: MonacoTheme.Space.sm) {
                        tile
                        labels
                    }
                    amount
                }
            }
            .frame(minHeight: MonacoButtonMetrics.hitTarget)
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("cabal-holdings-cash")
        .sheet(isPresented: $showsInfo) { CabalTreasuryApyInfoSheet() }
    }

    private var tile: some View {
        Image(systemName: MonacoIcon.card.symbol)
            .font(MonacoTheme.Typo.title2)
            .foregroundStyle(MonacoTheme.primaryText)
            .frame(width: HomeCashRowMetrics.tile, height: HomeCashRowMetrics.tile)
            .background(MonacoTheme.surfaceSunken, in: RoundedRectangle(cornerRadius: MonacoTheme.Radius.chip))
            .accessibilityHidden(true)
    }

    private var labels: some View {
        VStack(alignment: .leading, spacing: 0) {
            Text(cash.label)
                .font(MonacoTheme.Typo.headline)
                .foregroundStyle(MonacoTheme.primaryText)
            if LaunchFeatures.apy {
                HStack(spacing: 0) {
                    Text(cash.apy)
                        .font(MonacoTheme.Typo.subhead)
                        .foregroundStyle(MonacoTheme.secondaryText)
                    infoButton
                }
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private var amount: some View {
        Text(cash.amount)
            .moneyFigure(.row)
            .foregroundStyle(MonacoTheme.primaryText)
            .lineLimit(1)
            .minimumScaleFactor(MoneyStyle.row.minimumScaleFactor)
            .accessibilityLabel("Cash")
            .accessibilityValue(cash.amount)
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
        .accessibilityLabel("About \(cash.apy)")
        .accessibilityIdentifier("cabal-treasury-apy-info")
    }
}

private struct CabalTreasuryApyInfoSheet: View {
    var body: some View {
        Text(ProfileCopy.apyInfo)
            .font(MonacoTheme.Typo.subhead)
            .foregroundStyle(MonacoTheme.secondaryText)
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.bottom, MonacoTheme.Space.m)
            .frame(maxWidth: .infinity, alignment: .leading)
            .monacoFittedSheet(title: ProfileCopy.apyInfoTitle, showsClose: true)
            .accessibilityIdentifier("cabal-treasury-apy-info-sheet")
    }
}

struct CabalTreasuryActions: View {
    let cabalID: String
    let cashOut: String?
    let history: String?
    let open: (any AppRoute) -> Void

    @State private var showsChooser = false
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            MonacoCard(style: .inset) {
                VStack(spacing: 0) {
                    if CabalTreasuryBody.showsBotRow() { botRow }
                    if let cashOut { cashOutRow(cashOut) }
                    historyRow
                }
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            if cashOut != nil {
                Text(MoneyCopy.cashOutFootnote)
                    .font(MonacoTheme.Typo.subhead)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .fixedSize(horizontal: false, vertical: true)
                    .padding(.horizontal, MonacoTheme.Space.gutter + MonacoTheme.Space.xs)
                    .accessibilityIdentifier("cabal-treasury-footnote")
            }
        }
        .proposeChooser(isPresented: $showsChooser, cabalID: cabalID)
    }

    private var botRow: some View {
        MonacoRow(
            title: CabalTreasuryRows.botLabel, trailingIsInteractive: true,
            leading: { icon(MonacoIcon.bot.symbol) },
            trailing: {
                Toggle(CabalTreasuryRows.botLabel, isOn: botBinding)
                    .labelsHidden()
                    .accessibilityHint(CabalTreasuryCopy.botDetail)
                    .accessibilityIdentifier("cabal-treasury-bot-toggle")
            }
        )
        .accessibilityIdentifier("cabal-treasury-bot")
    }

    private var botBinding: Binding<Bool> {
        Binding(get: { false }, set: { _ in showsChooser = true })
    }

    private func cashOutRow(_ amount: String) -> some View {
        Button {
            Haptics.selection()
            open(CashOutRoute(cabalID: cabalID))
        } label: {
            MonacoRow(
                title: CabalTreasuryRows.cashOutLabel, chevron: true,
                leading: { icon(MonacoIcon.cashOut.symbol) },
                trailing: {
                    Text(amount)
                        .moneyFigure(.row, weight: .regular)
                        .foregroundStyle(MonacoTheme.secondaryText)
                        .lineLimit(1)
                }
            )
        }
        .buttonStyle(.monacoRow)
        .accessibilityIdentifier("cabal-treasury-cash-out")
    }

    private var historyRow: some View {
        Button {
            Haptics.selection()
            open(CabalTreasuryHistoryRoute(cabalID: cabalID))
        } label: {
            if dynamicTypeSize >= .xxxLarge {
                MonacoRow(
                    title: CabalTreasuryRows.historyLabel, subtitle: history, subtitleLineLimit: 6, chevron: true,
                    isLast: true,
                    leading: { icon(MonacoIcon.clock.symbol) },
                    trailing: { EmptyView() }
                )
            } else {
                MonacoRow(
                    title: CabalTreasuryRows.historyLabel, chevron: true, isLast: true,
                    leading: { icon(MonacoIcon.clock.symbol) },
                    trailing: {
                        if let history {
                            Text(history)
                                .font(MonacoTheme.Typo.rowTitle)
                                .foregroundStyle(MonacoTheme.secondaryText)
                                .lineLimit(1)
                        }
                    }
                )
            }
        }
        .buttonStyle(.monacoRow)
        .accessibilityIdentifier("cabal-treasury-history")
    }

    private func icon(_ symbol: String) -> some View {
        Image(systemName: symbol)
            .font(MonacoTheme.Typo.rowTitle)
            .foregroundStyle(MonacoTheme.secondaryText)
            .accessibilityHidden(true)
    }
}
