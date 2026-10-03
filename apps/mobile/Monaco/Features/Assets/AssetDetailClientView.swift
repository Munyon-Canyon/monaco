import MonacoCore
import SwiftUI

struct AssetDetailClientView: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    let symbol: String
    @State private var model: AssetDetailClientModel?
    @State private var selectedListing: String?
    private let position: AssetPositionSummary? = nil
    private let proposals: [AssetProposalDTO] = []
    private let activity: [AssetActivityDTO] = []
    private let holdings: [AssetHoldingDTO] = []
    private let proposeBuy: (() -> Void)? = nil

    var body: some View {
        Group {
            if let model { content(model) }
        }
        .navigationTitle(model?.detail?.ticker ?? AssetSymbolFormatter.display(symbol, kind: .stock))
        .navigationBarTitleDisplayMode(.inline)
        .task { await start() }
        .onChange(of: model?.failureTick) { _, _ in
            if let error = model?.lastError { toasts.show(error) }
        }
        .onChange(of: model?.chartError) { _, error in
            if let error { toasts.show(error) }
        }
        .navigationDestination(
            isPresented: Binding(
                get: { selectedListing != nil }, set: { if !$0 { selectedListing = nil } }
            )
        ) {
            if let selectedListing { AssetDetailClientView(symbol: selectedListing) }
        }
    }

    @ViewBuilder
    private func content(_ model: AssetDetailClientModel) -> some View {
        switch model.phase {
        case .idle where model.detail == nil, .loading where model.detail == nil:
            ProgressView().frame(maxWidth: .infinity, maxHeight: .infinity)
        case .failed where model.detail == nil:
            EmptyState(title: "Could not load this stock", actionTitle: "Retry") {
                Task { await model.load() }
            }
            .accessibilityIdentifier("asset-detail-failed")
        default:
            if let detail = model.detail { loaded(detail, model: model) }
        }
    }

    private func loaded(_ detail: AssetDetailPresentation, model: AssetDetailClientModel) -> some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.l) {
                hero(detail)
                chart(detail, model: model)
                Text(detail.attribution)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
                    .padding(.horizontal, MonacoTheme.Space.m)
                otherListings(detail.otherListings)
                if let position {
                    AssetPositionCard(summary: position, proposals: proposals, symbol: detail.symbol)
                }
                if !activity.isEmpty {
                    AssetActivityCard(symbol: detail.symbol, activity: activity)
                }
            }
            .padding(.vertical, MonacoTheme.Space.m)
        }
        .monacoCanvas()
        .foregroundStyle(MonacoTheme.ink)
        .accessibilityIdentifier("asset-detail-root")
        .safeAreaInset(edge: .bottom, spacing: 0) {
            if let proposeBuy {
                AssetTradeBar(
                    state: AssetTradeBarState.make(
                        isRoutable: detail.isTradable, holdings: holdings, holdingsState: .answered
                    ),
                    onBuy: proposeBuy, onSell: {}
                )
            }
        }
    }

    private func hero(_ detail: AssetDetailPresentation) -> some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            HStack(alignment: .firstTextBaseline, spacing: MonacoTheme.Space.s) {
                StockMark(symbol: detail.symbol, displayName: detail.name, assetKind: detail.kind, size: 44)
                VStack(alignment: .leading, spacing: 2) {
                    Text(detail.name).font(MonacoTheme.Typo.title).accessibilityIdentifier("asset-detail-name")
                    Text(detail.ticker).font(MonacoTheme.Typo.data).foregroundStyle(MonacoTheme.muted)
                }
            }
            if let price = detail.priceMicros {
                MoneyText(micros: price, style: .hero, voice: .market)
                    .accessibilityIdentifier("asset-detail-price")
            }
            if let change = detail.changeBasisPoints { PercentText(basisPoints: change, style: .row) }
            if detail.market.showsSessionChip,
                let session = MarketSessionCopy.chip(for: detail.market.status)
            {
                MarketSessionChip(session: session)
            }
        }
        .padding(.horizontal, MonacoTheme.Space.m)
    }

    private func chart(_ detail: AssetDetailPresentation, model: AssetDetailClientModel) -> some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            if let chart = model.chart,
                let series = SparklineSeries(usdcMicros: chart.points.map(\.priceUsdcMicros))
            {
                Sparkline(series: series, tone: chartTone(chart))
                    .frame(maxWidth: .infinity)
                    .frame(height: 200)
                    .accessibilityIdentifier("asset-detail-chart")
            } else if model.chart != nil {
                EmptyState(
                    title: detail.kind == .preIpo
                        ? "Chart appears once prices are recorded."
                        : "No price history for this window yet"
                )
                .accessibilityIdentifier("asset-detail-chart-empty")
            } else if model.chartError != nil {
                EmptyState(title: "Could not load price history", actionTitle: "Retry") {
                    Task { await model.loadChart(range: model.selectedRange) }
                }
                .accessibilityIdentifier("asset-detail-chart-failed")
            } else {
                ProgressView().frame(maxWidth: .infinity, minHeight: 120)
                    .accessibilityIdentifier("asset-detail-chart-loading")
            }
            ScrollView(.horizontal) {
                HStack(spacing: MonacoTheme.Space.s) {
                    ForEach(AssetChartRange.allCases, id: \.self) { range in
                        if model.selectedRange == range {
                            Button(range.label) { Task { await model.loadChart(range: range) } }
                                .buttonStyle(.monacoPrimary)
                                .accessibilityAddTraits(.isSelected)
                        } else {
                            Button(range.label) { Task { await model.loadChart(range: range) } }
                                .buttonStyle(.monacoSecondary)
                        }
                    }
                }
            }
            .accessibilityIdentifier("asset-chart-ranges")
        }
        .padding(.horizontal, MonacoTheme.Space.m)
    }

    private func chartTone(_ chart: AssetChartSeries) -> PnLTone {
        guard let first = chart.points.first?.priceUsdcMicros,
            let last = chart.points.last?.priceUsdcMicros
        else { return .flat }
        if last > first { return .profit }
        if last < first { return .loss }
        return .flat
    }

    @ViewBuilder
    private func otherListings(_ listings: [AssetListingPresentation]) -> some View {
        if !listings.isEmpty {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                MonacoSectionHeader("Also from")
                MonacoGroupedList {
                    ForEach(listings) { listing in
                        Button {
                            selectedListing = listing.symbol
                        } label: {
                            MonacoRow(
                                title: listing.name, subtitle: listing.ticker, chevron: true,
                                isLast: listing.id == listings.last?.id
                            ) {
                                EmptyView()
                            } trailing: {
                                if let issuer = listing.issuer.displayName {
                                    Text(issuer)
                                        .font(MonacoTheme.Typo.caption)
                                        .foregroundStyle(MonacoTheme.muted)
                                }
                            }
                        }
                        .buttonStyle(.monacoRow)
                        .accessibilityIdentifier("asset-other-listing-\(listing.symbol)")
                    }
                }
            }
            .padding(.horizontal, MonacoTheme.Space.m)
            .accessibilityIdentifier("asset-other-listings")
        }
    }

    private func start() async {
        if model == nil { model = AssetDetailClientModel(api: environment.api, symbol: symbol) }
        await model?.load()
    }
}
