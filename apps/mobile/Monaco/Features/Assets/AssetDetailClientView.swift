import MonacoAPI
import MonacoCore
import SwiftUI

struct AssetDetailClientView: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(\.hostMainTab) private var hostMainTab
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
                MonacoErrorRow(thing: "this stock", identifier: "asset-detail-failed") {
                    Task { await model.start() }
                }
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
                    .padding(.horizontal, MonacoTheme.Space.gutter)
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
                Text(detail.attribution)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                stats(detail, model: model)
                cabalPositions(model.positions)
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
            if let scrub = scrubHeader(model) {
                Text(scrub.price)
                    .moneyFont(.hero)
                    .lineLimit(1)
                    .minimumScaleFactor(MoneyStyle.hero.minimumScaleFactor)
                    .accessibilityIdentifier("asset-detail-price")
            } else if let price = detail.priceMicros {
                Text(UsdAmountFormatter.format(micros: price))
                    .moneyFont(.hero)
                    .lineLimit(1)
                    .minimumScaleFactor(MoneyStyle.hero.minimumScaleFactor)
                    .priceTickFlash(
                        priceTick,
                        in: RoundedRectangle(cornerRadius: MonacoTheme.Radius.chip, style: .continuous),
                        expand: MonacoTheme.Space.xs
                    )
                    .accessibilityIdentifier("asset-detail-price")
            } else {
                Text("—")
                    .moneyFont(.hero)
                    .foregroundStyle(MonacoTheme.muted)
                    .lineLimit(1)
                    .minimumScaleFactor(MoneyStyle.hero.minimumScaleFactor)
            }
            if let scrub = scrubHeader(model) {
                HStack(spacing: MonacoTheme.Space.xs) {
                    if let basisPoints = scrub.basisPoints {
                        PercentText(basisPoints: basisPoints, style: .row)
                    }
                    Text(scrub.caption)
                        .font(MonacoTheme.Typo.dataCaption)
                        .foregroundStyle(MonacoTheme.muted)
                }
                .accessibilityIdentifier("asset-detail-range-change")
            } else if let change = model.rangeChange {
                HStack(spacing: MonacoTheme.Space.xs) {
                    PercentText(basisPoints: change.basisPoints ?? 0, style: .row)
                    Text(change.label)
                        .font(MonacoTheme.Typo.dataCaption)
                        .foregroundStyle(MonacoTheme.muted)
                }
                .opacity(change.basisPoints == nil ? 0 : 1)
                .accessibilityHidden(change.basisPoints == nil)
                .accessibilityIdentifier("asset-detail-range-change")
            }
            if detail.market.showsSessionChip, let session = MarketSessionCopy.chip(for: detail.market.status) {
                MarketSessionChip(session: session)
                    .accessibilityIdentifier("asset-detail-session")
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
    }

    private func scrubHeader(_ model: AssetDetailClientModel) -> AssetScrubHeader? {
        AssetScrubHeader(chart: model.chart, index: scrubbedIndex)
    }

    private func chart(_ detail: AssetDetailPresentation, model: AssetDetailClientModel) -> some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            chartContent(detail, model: model)
            MonacoRangeChips(
                ranges: AssetChartRange.allCases, selection: model.selectedRange, identifierPrefix: "asset-range",
                label: { $0.label },
                onSelect: { range in Task { await model.loadChart(range: range) } }
            )
            .accessibilityIdentifier("asset-chart-ranges")
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
    }

    @ViewBuilder
    private func chartContent(_ detail: AssetDetailPresentation, model: AssetDetailClientModel) -> some View {
        switch model.chartPhase {
        case .idle, .loading:
            SkeletonBlock(width: nil, height: 200, radius: 0)
                .accessibilityIdentifier("asset-detail-chart-loading")
        case .failed:
            MonacoErrorRow(thing: "price history", identifier: "asset-detail-chart-failed") {
                Task { await model.loadChart(range: model.selectedRange) }
            }
        case .loaded:
            if let chart = model.chart, chart.points.count >= 2 {
                scrubChart(chart, isMarketLive: detail.session == .open)
            } else {
                EmptyState(
                    title: PreIpoCopy.chartEmpty,
                    message: detail.kind == .preIpo ? PreIpoCopy.chartEmptyMessage : "The curve draws as it trades."
                )
                .frame(height: 200)
                .accessibilityIdentifier("asset-detail-chart-empty")
            }
        }
    }

    private func scrubChart(_ chart: AssetChartSeries, isMarketLive: Bool) -> some View {
        MonacoScrubChart(
            points: chart.plotted,
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

    @ViewBuilder
    private func otherListings(_ listings: [AssetListingPresentation]) -> some View {
        if !listings.isEmpty {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                MonacoSectionHeader("Also available from")
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                MonacoGroupedList {
                    ForEach(listings) { listing in
                        Button {
                            environment.navigator.open(AssetRoute(symbol: listing.symbol), in: hostMainTab ?? .stocks)
                        } label: {
                            MonacoRow(
                                title: listing.name, subtitle: listing.ticker, chevron: true,
                                isLast: listing.id == listings.last?.id
                            ) {
                                StockMark(symbol: listing.symbol, displayName: listing.name, assetKind: listing.kind)
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
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier("asset-other-listings")
        }
    }

    private func start() async {
        if model == nil {
            model = AssetDetailClientModel(api: environment.api, symbol: symbol, hints: environment.hints)
        }
        await model?.start()
    }
}

extension AssetDetailClientView {
    @ViewBuilder
    fileprivate func stats(_ detail: AssetDetailPresentation, model: AssetDetailClientModel) -> some View {
        if let stats = model.stats {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                MonacoSectionHeader("Stats")
                MonacoGroupedList {
                    let rows = Self.statRows(stats)
                    ForEach(rows, id: \.label) { row in
                        ReceiptLine(label: row.label, value: .data(row.value), isLast: row.label == rows.last?.label)
                    }
                }
                if let basisPoints = stats.yearRangeBasisPoints(priceMicros: detail.priceMicros),
                    let low = stats.yearLow, let high = stats.yearHigh
                {
                    YearRangeBar(low: low, high: high, basisPoints: basisPoints)
                }
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier("asset-stats")
        }
    }

    fileprivate static func statRows(_ stats: AssetStats) -> [(label: String, value: String)] {
        [
            ("Open", stats.open), ("Day high", stats.dayHigh), ("Day low", stats.dayLow),
            ("Previous close", stats.previousClose), ("52-week high", stats.yearHigh), ("52-week low", stats.yearLow),
        ].compactMap { label, micros in micros.map { (label, UsdAmountFormatter.format(micros: $0)) } }
    }

    @ViewBuilder
    fileprivate func cabalPositions(_ positions: [AssetCabalPosition]) -> some View {
        if !positions.isEmpty {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                MonacoSectionHeader("Your cabals' position")
                MonacoGroupedList {
                    ForEach(positions) { position in
                        Button {
                            environment.navigator.open(CabalRoute(id: position.cabalID), in: hostMainTab ?? .stocks)
                        } label: {
                            MonacoRow(
                                title: position.cabalName, subtitle: position.sharesLabel, chevron: true,
                                isLast: position.id == positions.last?.id
                            ) {
                                CabalMark(
                                    groupId: position.cabalID, name: position.cabalName,
                                    pictureUrl: position.pictureURL)
                            } trailing: {
                                VStack(alignment: .trailing, spacing: MonacoTheme.Space.xs) {
                                    MoneyText(micros: position.valueMicros, style: .row)
                                    if let bps = position.returnBasisPoints {
                                        PnLBadge(signedMicros: position.pnlMicros, basisPoints: bps)
                                    }
                                }
                            }
                        }
                        .buttonStyle(.monacoRow)
                        .accessibilityIdentifier("asset-position-\(position.cabalID)")
                    }
                }
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier("asset-cabal-positions")
        }
    }

    fileprivate func proposeBar(_ detail: AssetDetailPresentation, model: AssetDetailClientModel) -> some View {
        BottomCTA {
            VStack(spacing: MonacoTheme.Space.xs) {
                HStack(spacing: MonacoTheme.Space.sm) {
                    Button(AssetDetailBuyCTA.title(tradable: detail.isTradable)) {
                        AssetDetailBuyCTA.open(symbol: symbol, kind: .buy, tab: hostMainTab ?? .stocks) {
                            environment.navigator.open($0, in: $1)
                        }
                    }
                    .buttonStyle(.monacoPrimary)
                    .disabled(!detail.isTradable)
                    .accessibilityIdentifier("asset-detail-propose-buy")
                    if model.heldByVotingCabal {
                        Button("Propose sell") {
                            AssetDetailBuyCTA.open(symbol: symbol, kind: .sell, tab: hostMainTab ?? .stocks) {
                                environment.navigator.open($0, in: $1)
                            }
                        }
                        .buttonStyle(.monacoSecondary)
                        .accessibilityIdentifier("asset-detail-propose-sell")
                    }
                }
                Text(AssetDetailBuyCTA.caption(tradable: detail.isTradable, canSell: model.heldByVotingCabal))
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
            }
        }
    }
}

private struct YearRangeBar: View {
    let low: Int64
    let high: Int64
    let basisPoints: Int64

    var body: some View {
        VStack(spacing: MonacoTheme.Space.xs) {
            GeometryReader { proxy in
                ZStack(alignment: .leading) {
                    Capsule().fill(MonacoTheme.surfaceSunken)
                    Circle()
                        .fill(MonacoTheme.brand)
                        .frame(width: 12, height: 12)
                        .offset(x: (proxy.size.width - 12) * CGFloat(basisPoints) / 10_000)
                }
            }
            .frame(height: 12)
            HStack {
                Text(UsdAmountFormatter.format(micros: low))
                Spacer()
                Text(UsdAmountFormatter.format(micros: high))
            }
            .font(MonacoTheme.Typo.dataCaption)
            .foregroundStyle(MonacoTheme.muted)
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(
            "52-week range, \(UsdAmountFormatter.format(micros: low)) to \(UsdAmountFormatter.format(micros: high))"
        )
        .accessibilityIdentifier("asset-year-range")
    }
}

enum AssetDetailBuyCTA {
    static func title(tradable: Bool) -> String {
        tradable ? "Propose buy" : "Can't buy right now"
    }

    static func caption(tradable: Bool, canSell: Bool) -> String {
        if !tradable { return "Not available to buy yet." }
        if canSell { return "Your cabal votes before anything is bought or sold" }
        return "Your cabal votes before anything is bought"
    }

    static func open(
        symbol: String, kind: ProposeKind, tab: MainTab, navigator: (ProposeFromAssetRoute, MainTab) -> Void
    ) {
        navigator(ProposeFromAssetRoute(symbol: symbol, kind: kind), tab)
    }
}

#if DEBUG
private enum AssetDetailClientSampleScenario: String {
    case open, previousClose, sparse, untradable, fallbackSeries, emptyChart, chartFailed
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
        let previousClose: Int64? = scenario == .previousClose ? 176_000_000 : nil
        let chart =
            scenario == .emptyChart || scenario == .chartFailed
            ? nil : Self.chart(range: range, previousClose: previousClose)
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

    private static func chart(range: AssetChartRange, previousClose: Int64?) -> AssetChartSeries {
        AssetChartSeries(
            range: range,
            points: [
                .init(timestamp: 1_772_596_200, priceUsdcMicros: 174_000_000),
                .init(timestamp: 1_772_614_200, priceUsdcMicros: 175_420_000),
            ],
            previousCloseUsdcMicros: previousClose
        )
    }
}
#endif

struct AssetScrubHeader: Equatable {
    let price: String
    let basisPoints: Int64?
    let caption: String

    init?(
        chart: AssetChartSeries?,
        index: Int?,
        locale: Locale = .autoupdatingCurrent,
        timeZone: TimeZone = .autoupdatingCurrent
    ) {
        guard let chart, let index, let point = chart.point(at: index) else { return nil }
        price = UsdAmountFormatter.format(micros: point.priceUsdcMicros)
        basisPoints = Self.basisPoints(from: chart.baselineUsdcMicros, to: point.priceUsdcMicros)
        caption = ChartScrubLabel.caption(
            for: Date(timeIntervalSince1970: TimeInterval(point.timestamp)),
            range: chart.range, locale: locale, timeZone: timeZone)
    }

    private static func basisPoints(from first: Int64?, to value: Int64) -> Int64? {
        guard let first, first > 0 else { return nil }
        let (change, changeOverflow) = value.subtractingReportingOverflow(first)
        let (scaled, scaleOverflow) = change.multipliedReportingOverflow(by: 10_000)
        guard !changeOverflow, !scaleOverflow else { return nil }
        return scaled / first
    }
}
