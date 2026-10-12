import MonacoAPI
import MonacoCore
import SwiftUI

struct AssetDetailClientView: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(\.hostMainTab) private var hostMainTab
    let symbol: String
    @State private var model: AssetDetailClientModel?
    @State private var scrubbedIndex: Int?
    @AppStorage("asset-chart-style") private var storedStyle = MonacoChartStyle.candles.rawValue
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
            .task { await environment.bestChains.refreshIfNeeded(symbol: symbol) }
            .onScreenVisibilityChange { visible in
                model?.setVisible(visible)
                guard visible else { return }
                Task { await environment.bestChains.refreshIfNeeded(symbol: symbol) }
            }
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
        .monacoFrameStats("AssetDetail")
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
            if let bestChain = environment.bestChains.bestChain(for: symbol) {
                AssetBestChainLine(bestChain: bestChain)
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
    }

    private func scrubHeader(_ model: AssetDetailClientModel) -> AssetScrubHeader? {
        AssetScrubHeader(chart: model.chart, barIndex: scrubbedIndex)
    }

    private func chart(_ detail: AssetDetailPresentation, model: AssetDetailClientModel) -> some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            chartControls(model)
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
    fileprivate var chartStyle: Binding<MonacoChartStyle> {
        Binding(
            get: { MonacoChartStyle(rawValue: storedStyle) ?? .candles },
            set: { storedStyle = $0.rawValue }
        )
    }

    @ViewBuilder
    fileprivate func chartControls(_ model: AssetDetailClientModel) -> some View {
        if model.chartPhase == .loaded, model.chart?.isDrawable == true {
            HStack(spacing: MonacoTheme.Space.s) {
                intervalMenu(model)
                Spacer(minLength: 0)
                MonacoChartStyleToggle(style: chartStyle)
            }
        }
    }

    fileprivate func intervalMenu(_ model: AssetDetailClientModel) -> some View {
        Menu {
            ForEach(ChartInterval.allowed(for: model.selectedRange), id: \.self) { interval in
                Button {
                    guard interval != model.selectedInterval else { return }
                    Haptics.selection()
                    scrubbedIndex = nil
                    Task { await model.loadChart(range: model.selectedRange, interval: interval) }
                } label: {
                    if interval == model.selectedInterval {
                        Label(interval.label, systemImage: "checkmark")
                    } else {
                        Text(interval.label)
                    }
                }
                .accessibilityIdentifier("asset-interval-\(interval.rawValue)")
            }
        } label: {
            HStack(spacing: MonacoTheme.Space.xs) {
                Text(model.selectedInterval.label)
                Image(systemName: "chevron.down").font(MonacoTheme.Typo.caption)
            }
            .monacoChipLabel(isSelected: false, isPreset: true)
        }
        .accessibilityLabel("Bar interval, \(model.selectedInterval.label)")
        .accessibilityIdentifier("asset-chart-interval")
    }

    @ViewBuilder
    fileprivate func chartContent(_ detail: AssetDetailPresentation, model: AssetDetailClientModel) -> some View {
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
                candleChart(chart, model: model, isMarketLive: detail.session == .open)
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

    fileprivate func drawnBars(_ chart: AssetChartSeries) -> [PlottedBar] {
        #if DEBUG
        if ProcessInfo.processInfo.arguments.contains("-MonacoRawCandles") { return chart.bars }
        #endif
        return chart.candleBars
    }

    fileprivate func candleChart(
        _ chart: AssetChartSeries, model: AssetDetailClientModel, isMarketLive: Bool
    ) -> some View {
        MonacoCandleChart(
            bars: drawnBars(chart),
            intervalSeconds: chart.intervalSeconds,
            style: chartStyle.wrappedValue,
            tint: chartTone(chart),
            lastPrice: chart.lastPoint?.chartValue,
            isLive: isMarketLive && chart.range == .oneDay,
            resetKey: "\(chart.range.rawValue)-\(chart.interval.rawValue)",
            hasOlder: chart.hasOlder && chart.bars.count < AssetDetailClientModel.maximumBars,
            retryToken: model.olderFailureTick,
            selection: $scrubbedIndex,
            onNeedOlder: { Task { await model.loadOlder() } },
            summary: "\(chart.range.label) price history, \(chart.interval.label) bars",
            nearestIndex: { chart.nearestBarIndex(to: $0) },
            describeBar: { chart.describeBar(at: $0) ?? "" },
            accessibilityIdentifier: "asset-detail-chart"
        )
    }

    fileprivate func chartTone(_ chart: AssetChartSeries) -> Color {
        guard let basisPoints = model?.rangeChange?.basisPoints else { return MonacoTheme.muted }
        return basisPoints > 0 ? MonacoTheme.profitVivid : basisPoints < 0 ? MonacoTheme.lossVivid : MonacoTheme.muted
    }

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
                    .disabled(!AssetDetailBuyCTA.canBuy(tradable: detail.isTradable, quotable: detail.isQuotable))
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
                Text(
                    AssetDetailBuyCTA.caption(
                        tradable: detail.isTradable, quotable: detail.isQuotable, canSell: model.heldByVotingCabal)
                )
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
            }
        }
    }
}

private struct AssetBestChainLine: View {
    let bestChain: AssetBestChain

    var body: some View {
        HStack(spacing: MonacoTheme.Space.xs) {
            ChainMark(network: bestChain.network.rawValue, size: 24)
            Text(bestChain.headline)
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.secondaryText)
                .fixedSize(horizontal: false, vertical: true)
        }
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("asset-detail-best-chain")
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

    static func canBuy(tradable: Bool, quotable: Bool) -> Bool { tradable && quotable }

    static func caption(tradable: Bool, quotable: Bool = true, canSell: Bool) -> String {
        if !tradable { return "Not available to buy yet." }
        if !quotable { return "Trading paused right now" }
        if canSell { return "Your cabal votes before anything is bought or sold" }
        return "Your cabal votes before anything is bought"
    }

    static func open(
        symbol: String, kind: ProposeKind, tab: MainTab, navigator: (ProposeFromAssetRoute, MainTab) -> Void
    ) {
        navigator(ProposeFromAssetRoute(symbol: symbol, kind: kind), tab)
    }
}

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

    init?(
        chart: AssetChartSeries?,
        barIndex: Int?,
        locale: Locale = .autoupdatingCurrent,
        timeZone: TimeZone = .autoupdatingCurrent
    ) {
        guard let chart, let barIndex, let point = chart.barPoint(at: barIndex) else { return nil }
        price = UsdAmountFormatter.format(micros: point.priceUsdcMicros)
        basisPoints = Self.basisPoints(from: chart.baselineUsdcMicros, to: point.priceUsdcMicros)
        caption = ChartScrubLabel.caption(
            for: point.date, range: chart.range, intervalSeconds: chart.intervalSeconds,
            locale: locale, timeZone: timeZone)
    }

    private static func basisPoints(from first: Int64?, to value: Int64) -> Int64? {
        guard let first, first > 0 else { return nil }
        let (change, changeOverflow) = value.subtractingReportingOverflow(first)
        let (scaled, scaleOverflow) = change.multipliedReportingOverflow(by: 10_000)
        guard !changeOverflow, !scaleOverflow else { return nil }
        return scaled / first
    }
}
