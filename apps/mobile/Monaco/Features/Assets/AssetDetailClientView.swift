import MonacoAPI
import MonacoCore
import SwiftUI

struct AssetDetailClientView: View {
    @Environment(AppEnvironment.self) private var environment
    let symbol: String
    @State private var model: AssetDetailClientModel?
    @State private var scrubbedIndex: Int?
    @State private var priceTick: MonacoPriceTick?

    init(symbol: String, model: AssetDetailClientModel? = nil) {
        self.symbol = symbol
        _model = State(initialValue: model)
    }

    var body: some View {
        content
            .navigationTitle(model?.detail?.ticker ?? AssetSymbolFormatter.display(symbol, kind: .stock))
            .navigationBarTitleDisplayMode(.inline)
            .task {
                await start()
                await model?.loadHeldByVotingCabal()
                await model?.observe()
            }
            .onScreenVisibilityChange { model?.setVisible($0) }
            .onChange(of: model?.detail?.priceMicros) { oldPrice, newPrice in
                guard let oldPrice, let newPrice, oldPrice != newPrice else { return }
                priceTick = .init(sequence: (priceTick?.sequence ?? 0) + 1, isUp: newPrice > oldPrice)
            }
    }

    @ViewBuilder
    private var content: some View {
        if let model {
            switch model.detailPhase {
            case .idle, .loading:
                loading
            case .failed:
                EmptyState(title: "Couldn't load this stock.", actionTitle: "Try again") {
                    Task { await model.load() }
                }
                .accessibilityIdentifier("asset-detail-failed")
            case .loaded:
                if let detail = model.detail { loaded(detail, model: model) }
            }
        } else {
            loading
        }
    }

    private var loading: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.l) {
                AssetDetailHeroSkeleton()
                    .padding(.horizontal, MonacoTheme.Space.m)
                SkeletonBlock(width: nil, height: 200, radius: 0)
            }
            .padding(.vertical, MonacoTheme.Space.m)
        }
        .monacoCanvas()
        .accessibilityIdentifier("asset-detail-loading")
    }

    private func loaded(_ detail: AssetDetailPresentation, model: AssetDetailClientModel) -> some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.l) {
                hero(detail, model: model)
                chart(detail, model: model)
                if model.isShortHistory { shortHistory }
                Text(detail.attribution)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
                    .padding(.horizontal, MonacoTheme.Space.m)
                otherListings(detail.otherListings)
            }
            .padding(.vertical, MonacoTheme.Space.m)
            .padding(.bottom, MonacoTheme.Space.xl + 96)
        }
        .monacoCanvas()
        .foregroundStyle(MonacoTheme.ink)
        .safeAreaInset(edge: .bottom) { proposeBar(detail, model: model) }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("asset-detail-root")
    }

    private func hero(_ detail: AssetDetailPresentation, model: AssetDetailClientModel) -> some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            Text(detail.name)
                .font(MonacoTheme.Typo.title)
                .accessibilityIdentifier("asset-detail-name")
            Text(detail.ticker)
                .font(MonacoTheme.Typo.ticker)
                .foregroundStyle(MonacoTheme.muted)
            if let price = detail.priceMicros {
                Text(UsdAmountFormatter.format(micros: price))
                    .font(MonacoTheme.Typo.quoteHero)
                    .priceTickFlash(
                        priceTick,
                        in: RoundedRectangle(cornerRadius: MonacoTheme.Radius.chip, style: .continuous),
                        expand: MonacoTheme.Space.xs
                    )
                    .accessibilityIdentifier("asset-detail-price")
            }
            if let change = model.rangeChange {
                HStack(spacing: MonacoTheme.Space.xs) {
                    if let basisPoints = change.basisPoints {
                        PercentText(basisPoints: basisPoints, style: .row)
                    }
                    Text(change.label)
                        .font(MonacoTheme.Typo.dataCaption)
                        .foregroundStyle(MonacoTheme.muted)
                }
                .accessibilityIdentifier("asset-detail-range-change")
            }
            if let session = MarketSessionCopy.chip(for: detail.market.status) {
                MarketSessionChip(session: session)
                    .accessibilityIdentifier("asset-detail-session")
            }
            Text("Trading 24/7 on Solana")
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
        }
        .padding(.horizontal, MonacoTheme.Space.m)
    }

    private func chart(_ detail: AssetDetailPresentation, model: AssetDetailClientModel) -> some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            chartContent(detail, model: model)
            ScrollView(.horizontal) {
                HStack(spacing: MonacoTheme.Space.s) {
                    ForEach(AssetChartRange.allCases, id: \.self) { range in
                        rangeButton(range, model: model)
                    }
                }
            }
            .scrollIndicators(.hidden)
            .accessibilityIdentifier("asset-chart-ranges")
        }
        .padding(.horizontal, MonacoTheme.Space.m)
    }

    @ViewBuilder
    private func rangeButton(_ range: AssetChartRange, model: AssetDetailClientModel) -> some View {
        if model.selectedRange == range {
            Button(range.label) { Task { await model.loadChart(range: range) } }
                .buttonStyle(.monacoPrimary)
                .accessibilityAddTraits(.isSelected)
        } else {
            Button(range.label) { Task { await model.loadChart(range: range) } }
                .buttonStyle(.monacoSecondary)
        }
    }

    @ViewBuilder
    private func chartContent(_ detail: AssetDetailPresentation, model: AssetDetailClientModel) -> some View {
        switch model.chartPhase {
        case .idle, .loading:
            SkeletonBlock(width: nil, height: 200, radius: 0)
                .accessibilityIdentifier("asset-detail-chart-loading")
        case .failed:
            EmptyState(title: "Couldn't load price history.", actionTitle: "Try again") {
                Task { await model.loadChart(range: model.selectedRange) }
            }
            .accessibilityIdentifier("asset-detail-chart-failed")
        case .loaded:
            if let chart = model.chart, chart.points.count >= 2 {
                scrubChart(chart, isMarketLive: detail.session == .open)
            } else {
                EmptyState(
                    title: "Price history builds up over time.",
                    message: "The curve draws as the token trades."
                )
                .accessibilityIdentifier("asset-detail-chart-empty")
            }
        }
    }

    private func scrubChart(_ chart: AssetChartSeries, isMarketLive: Bool) -> some View {
        MonacoScrubChart(
            points: chart.points.map { .init(date: $0.date, value: $0.chartValue) },
            tint: chartTone(chart),
            baseline: chart.drawsBaselineRule ? chart.baselineValue : nil,
            isLive: isMarketLive && chart.range == .oneDay,
            drawOnKey: chart.range.rawValue,
            selection: $scrubbedIndex,
            summary: "\(chart.range.label) price history",
            nearestIndex: { chart.nearestIndex(to: $0) },
            describePoint: { index in
                guard let point = chart.point(at: index) else { return "" }
                return UsdAmountFormatter.format(micros: point.priceUsdcMicros)
            },
            accessibilityIdentifier: "asset-detail-chart"
        )
    }

    private func chartTone(_ chart: AssetChartSeries) -> Color {
        guard let basisPoints = model?.rangeChange?.basisPoints else { return MonacoTheme.muted }
        return basisPoints > 0 ? MonacoTheme.profitVivid : basisPoints < 0 ? MonacoTheme.lossVivid : MonacoTheme.muted
    }

    private var shortHistory: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
            Text("Price history builds up over time.").font(MonacoTheme.Typo.bodyStrong)
            Text("The curve draws as the token trades.")
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
        }
        .padding(.horizontal, MonacoTheme.Space.m)
        .accessibilityIdentifier("asset-detail-short-history")
    }

    @ViewBuilder
    private func otherListings(_ listings: [AssetListingPresentation]) -> some View {
        if !listings.isEmpty {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                MonacoSectionHeader("Also available from")
                MonacoGroupedList {
                    ForEach(listings) { listing in
                        Button {
                            environment.navigator.open(AssetRoute(symbol: listing.symbol), in: .stocks)
                        } label: {
                            MonacoRow(
                                title: listing.name, subtitle: listing.ticker, chevron: true,
                                isLast: listing.id == listings.last?.id
                            ) {
                                EmptyView()
                            } trailing: {
                                if let issuer = listing.issuer.displayName {
                                    Text(issuer).font(MonacoTheme.Typo.caption).foregroundStyle(MonacoTheme.muted)
                                }
                            }
                        }
                        .buttonStyle(.monacoRow)
                        .accessibilityIdentifier("asset-other-listing-\(listing.symbol)")
                    }
                }
            }
            .padding(.horizontal, MonacoTheme.Space.m)
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier("asset-other-listings")
        }
    }

    private func start() async {
        if model == nil {
            model = AssetDetailClientModel(api: environment.api, symbol: symbol, hints: environment.hints)
        }
        await model?.load()
    }
}

extension AssetDetailClientView {
    fileprivate func proposeBar(_ detail: AssetDetailPresentation, model: AssetDetailClientModel) -> some View {
        BottomCTA {
            VStack(spacing: MonacoTheme.Space.xs) {
                HStack(spacing: MonacoTheme.Space.sm) {
                    Button(AssetDetailBuyCTA.title(tradable: detail.isTradable)) {
                        AssetDetailBuyCTA.open(symbol: symbol, kind: .buy) {
                            environment.navigator.open($0, in: $1)
                        }
                    }
                    .buttonStyle(.monacoPrimary)
                    .disabled(!detail.isTradable)
                    .accessibilityIdentifier("asset-detail-propose-buy")
                    if model.heldByVotingCabal {
                        Button("Propose sell") {
                            AssetDetailBuyCTA.open(symbol: symbol, kind: .sell) {
                                environment.navigator.open($0, in: $1)
                            }
                        }
                        .buttonStyle(.monacoSecondary)
                        .accessibilityIdentifier("asset-detail-propose-sell")
                    }
                }
                Text("Your cabal votes before anything is bought")
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
            }
        }
    }
}

enum AssetDetailBuyCTA {
    static func title(tradable: Bool) -> String {
        tradable ? "Propose buy" : "Can't buy right now"
    }

    static func open(symbol: String, kind: ProposeKind, navigator: (ProposeFromAssetRoute, MainTab) -> Void) {
        navigator(ProposeFromAssetRoute(symbol: symbol, kind: kind), .stocks)
    }
}

#if DEBUG
private enum AssetDetailClientSampleScenario: String {
    case open, sparse, untradable, fallbackSeries, emptyChart, chartFailed
    static func matching(_ arguments: [String]) -> Self? {
        guard let index = arguments.firstIndex(of: "-MonacoAssetDetailSample"), arguments.indices.contains(index + 1)
        else { return nil }
        return Self(rawValue: arguments[index + 1])
    }
}
final class AssetDetailClientSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        guard let scenario = AssetDetailClientSampleScenario.matching(arguments) else { return nil }
        return AnyView(AssetDetailClientSampleHarness(scenario: scenario))
    }
}
private struct AssetDetailClientSampleHarness: View {
    let scenario: AssetDetailClientSampleScenario
    @State private var model: AssetDetailClientModel
    init(scenario: AssetDetailClientSampleScenario) {
        self.scenario = scenario
        let isPreIpo = scenario == .sparse
        var detail = isPreIpo ? Components.Schemas.AssetDetail.spaceX : .googl
        if scenario == .untradable { detail.tradable = false }
        let range: AssetChartRange = scenario == .fallbackSeries ? .oneYear : .oneDay
        let chart = scenario == .emptyChart || scenario == .chartFailed ? nil : Self.chart(range: range)
        _model = State(
            initialValue: AssetDetailClientModel(
                sampleDetail: detail,
                chart: chart,
                selectedRange: range,
                chartPhase: scenario == .chartFailed ? .failed(APIError(URLError(.cannotLoadFromNetwork))) : .loaded
            ))
    }

    var body: some View {
        NavigationStack { AssetDetailClientView(symbol: model.detail?.symbol ?? "", model: model) }
            .tint(MonacoTheme.ink)
    }

    private static func chart(range: AssetChartRange) -> AssetChartSeries {
        AssetChartSeries(
            range: range,
            points: [
                .init(timestamp: 1_772_596_200, priceUsdcMicros: 174_000_000),
                .init(timestamp: 1_772_614_200, priceUsdcMicros: 175_420_000),
            ]
        )
    }
}
#endif
