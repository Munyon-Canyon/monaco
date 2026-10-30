import MonacoAPI
import MonacoCore
import SwiftUI

struct CabalHoldingView: View {
    let cabalID: String
    let symbol: String

    @Environment(AppEnvironment.self) private var environment
    @State private var model: CabalHoldingModel?

    var body: some View {
        Group {
            if let model {
                CabalHoldingScreen(model: model) { route in
                    environment.navigator.open(route, in: environment.navigator.selectedTab)
                }
            } else {
                HoldingSkeleton().frame(maxHeight: .infinity, alignment: .top).monacoCanvas()
            }
        }
        .task {
            let model = prepared()
            await model.load()
        }
    }

    private func prepared() -> CabalHoldingModel {
        if let model { return model }
        let created = CabalHoldingModel(cabalID: cabalID, symbol: symbol, api: environment.api) { Date() }
        model = created
        return created
    }
}

struct CabalHoldingScreen: View {
    let model: CabalHoldingModel
    let open: (any AppRoute) -> Void

    @Environment(ToastCenter.self) private var toasts
    @State private var selection: Int?

    var body: some View {
        content
            .monacoCanvas()
            .navigationTitle(model.navigationTitle)
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                if case .loaded = model.outcome {
                    ToolbarItem(placement: .topBarTrailing) {
                        MonacoGlassButton(
                            icon: .share, accessibilityLabel: "Share position", style: .quiet, placement: .toolbar
                        ) {
                            open(ShareRoute(.position(cabalID: model.cabalID, symbol: model.symbol)))
                        }
                        .accessibilityIdentifier("holding-share")
                    }
                }
            }
            .onChange(of: model.toast) { _, message in
                guard let message else { return }
                toasts.current = MonacoToast(message: message)
                model.dismissToast()
            }
            .accessibilityIdentifier("holding-screen")
    }

    @ViewBuilder private var content: some View {
        switch model.outcome {
        case .loading:
            HoldingSkeleton().frame(maxHeight: .infinity, alignment: .top)
        case .failed:
            EmptyState(
                title: CabalHoldingCopy.errorTitle, message: CabalHoldingCopy.errorBody,
                actionTitle: CabalHoldingCopy.tryAgain, actionIdentifier: "holding-retry",
                systemImage: MonacoIcon.offline.symbol
            ) { Task { await model.load() } }
            .frame(maxHeight: .infinity)
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier("holding-error")
        case .notMember:
            EmptyState(
                title: CabalHoldingCopy.privateTitle,
                message: CabalHoldingCopy.privateBody(model.cabalName.isEmpty ? "this cabal" : model.cabalName),
                systemImage: MonacoIcon.lock.symbol
            )
            .frame(maxHeight: .infinity)
            .accessibilityElement(children: .combine)
            .accessibilityIdentifier("holding-private")
        case .loaded(let holding):
            loaded(holding)
        case .empty(let holding):
            loaded(holding)
        }
    }

    private func loaded(_ holding: CabalHolding) -> some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 0) {
                header(holding)
                if let minutes = holding.staleMinutes {
                    MonacoNotice(.warn, message: CabalHoldingCopy.stale(minutes))
                        .accessibilityIdentifier("holding-stale")
                }
                performance(holding)
                chart(holding)
                HoldingStatGrid(stats: holding.stats)
                if holding.hasPendingFigures {
                    Text(CabalHoldingCopy.opensSoon)
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.secondaryText)
                        .padding(.top, MonacoTheme.Space.s)
                        .accessibilityIdentifier("holding-opens-soon")
                }
                trades(holding)
                related(holding)
                HoldingDetails(holding: holding) { open(AssetRoute(symbol: holding.symbol)) }
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.bottom, MonacoTheme.Space.l)
        }
        .refreshable { await model.load() }
        .scrollBounceBehavior(.basedOnSize)
    }

    private func header(_ holding: CabalHolding) -> some View {
        HStack(spacing: MonacoTheme.Space.sm) {
            StockMark(
                symbol: holding.symbol, displayName: holding.assetName, assetKind: holding.assetKind,
                size: StockMarkSize.header, logoURL: holding.logoURL)
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                Text(holding.assetName)
                    .font(MonacoTheme.Typo.section)
                    .foregroundStyle(MonacoTheme.primaryText)
                Text(holding.header)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.secondaryText)
            }
            Spacer(minLength: 0)
        }
        .padding(.vertical, MonacoTheme.Space.l)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(holding.headerSpoken)
        .accessibilityAddTraits(.isHeader)
        .accessibilityIdentifier("holding-header")
    }

    private func performance(_ holding: CabalHolding) -> some View {
        let period = model.period
        return VStack(alignment: .leading, spacing: 0) {
            Text(CabalHoldingCopy.positionValue)
                .font(MonacoTheme.Typo.captionStrong)
                .foregroundStyle(MonacoTheme.secondaryText)
                .padding(.bottom, MonacoTheme.Space.s)
            MoneyText(micros: holding.valueMicros, style: .hero, color: MonacoTheme.primaryText)
                .accessibilityIdentifier("holding-value")
            HStack(spacing: MonacoTheme.Space.sm) {
                returnFigure(holding, period: period)
                    .accessibilityIdentifier("holding-return")
                Text(holding.returnLabel(for: period))
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.secondaryText)
            }
            .padding(.top, MonacoTheme.Space.xs)
        }
        .padding(.top, MonacoTheme.Space.gutter)
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityElement(children: .combine)
    }

    @ViewBuilder private func returnFigure(_ holding: CabalHolding, period: HoldingPeriod) -> some View {
        if holding.isEmpty {
            MoneyText(micros: 0, style: .row, color: MonacoTheme.profit)
        } else {
            PnLText(signedMicros: holding.returnMicros(for: period), style: .row)
        }
    }

    @ViewBuilder private func chart(_ holding: CabalHolding) -> some View {
        if holding.isEmpty {
            EmptyState(
                title: CabalHoldingCopy.emptyTitle, message: CabalHoldingCopy.emptyBody,
                systemImage: "chart.line.uptrend.xyaxis"
            )
            .accessibilityElement(children: .combine)
            .accessibilityIdentifier("holding-empty")
        } else if let curve = holding.curve(for: model.period) {
            VStack(spacing: MonacoTheme.Space.s) {
                CurveScrubChart(
                    curve: curve, plot: .pnl, range: model.period.chartRange, selection: $selection,
                    identifier: "holding-chart")
                MonacoRangeChips(
                    ranges: HoldingPeriod.allCases, selection: model.period, identifierPrefix: "holding-period",
                    label: \.label
                ) { period in
                    selection = nil
                    model.period = period
                }
            }
            .padding(.top, MonacoTheme.Space.m)
            .padding(.bottom, MonacoTheme.Space.m)
        } else {
            QuietChart(message: CabalHoldingCopy.chartOpensSoon)
                .padding(.vertical, MonacoTheme.Space.m)
                .accessibilityIdentifier("holding-chart-quiet")
        }
    }

    @ViewBuilder private func trades(_ holding: CabalHolding) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            MonacoSectionHeader(CabalHoldingCopy.tradeHistory)
            Text(CabalHoldingCopy.utc(holding.cabalName))
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
                .padding(.top, MonacoTheme.Space.xs)
                .padding(.bottom, MonacoTheme.Space.sm)
            switch model.trades {
            case .loading:
                MonacoRowSkeleton(rows: 2, markShape: .circle)
                    .accessibilityIdentifier("holding-trades-loading")
            case .failed:
                MonacoErrorRow(thing: "trade history", identifier: "holding-trades-error", inset: false) {
                    Task { await model.retryTrades() }
                }
            case .loaded(let trades, let hasMore):
                if trades.isEmpty {
                    Text(CabalHoldingCopy.noTrades)
                        .font(MonacoTheme.Typo.subhead)
                        .foregroundStyle(MonacoTheme.secondaryText)
                        .padding(.vertical, MonacoTheme.Space.s)
                        .accessibilityIdentifier("holding-no-trades")
                }
                ForEach(trades) { trade in
                    Button {
                        open(TransactionRoute(cabalID: holding.cabalID, transactionID: trade.id))
                    } label: {
                        HoldingTradeRow(trade: trade)
                    }
                    .buttonStyle(.monacoRow)
                }
                if hasMore {
                    Button(CabalHoldingCopy.olderTrades) { Task { await model.loadMoreTrades() } }
                        .buttonStyle(.monacoLoadMore)
                        .disabled(model.isLoadingMoreTrades)
                        .accessibilityIdentifier("holding-older-trades")
                }
            }
        }
        .padding(.top, MonacoTheme.Space.l)
    }

    @ViewBuilder private func related(_ holding: CabalHolding) -> some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
            MonacoSectionHeader(CabalHoldingCopy.related)
            if model.proposals.isEmpty {
                MonacoQuietEmpty(
                    CabalHoldingCopy.noProposals(holding.ticker), systemImage: MonacoIcon.checkCircle.symbol
                )
                .accessibilityIdentifier("holding-no-proposals")
            }
            ForEach(model.proposals) { proposal in
                ProposalCard(
                    proposal: proposal, asset: model.proposalAsset, members: model.members,
                    openRoute: AnyAppRoute(ProposalRoute(proposalID: proposal.id)))
            }
        }
        .padding(.top, MonacoTheme.Space.l)
    }
}
