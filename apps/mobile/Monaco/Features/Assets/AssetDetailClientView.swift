import MonacoAPI
import MonacoCore
import SwiftUI

struct AssetDetailClientView: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
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

    private var tab: MainTab { hostMainTab ?? .stocks }

    var body: some View {
        content
            .navigationTitle(model?.detail?.name ?? "")
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
                AssetsLoadFailure(identifier: "asset-detail-failed") { Task { await model.start() } }
                    .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
                    .monacoCanvas()
            case .loaded:
                if let detail = model.detail { loaded(detail, model: model) }
            }
        } else {
            loading
        }
    }

    private var loading: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                AssetDetailHeroSkeleton()
                MonacoRowSkeleton(rows: 4, markShape: .circle, hasTrailing: false)
                    .monacoBleed()
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.top, MonacoTheme.Space.m)
        }
        .monacoCanvas()
        .accessibilityIdentifier("asset-detail-loading")
    }

    private func loaded(_ detail: AssetDetailPresentation, model: AssetDetailClientModel) -> some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.section) {
                AssetDetailHeader(
                    detail: detail, model: model,
                    scrub: AssetScrubHeader(chart: model.chart, barIndex: scrubbedIndex), priceTick: priceTick,
                    bestChain: environment.bestChains.bestChain(for: symbol),
                    isWatched: environment.watchlist.isWatched(symbol), toggleWatch: toggleWatch)
                AssetDetailChartSection(
                    detail: detail, model: model, scrubbedIndex: $scrubbedIndex, style: chartStyle)
                AssetDetailActions(detail: detail, canSell: model.heldByVotingCabal, propose: propose)
                AssetMarketStats(
                    stats: model.stats, attribution: detail.attribution, priceMicros: detail.priceMicros,
                    volume24hMicros: detail.market.volume24hMicros)
                AssetAbout(detail: detail)
                AssetCabalHoldings(ticker: detail.ticker, positions: model.positions) {
                    environment.navigator.open(CabalRoute(id: $0.cabalID), in: tab)
                }
                AssetProposals(ticker: detail.ticker)
                if !detail.otherListings.isEmpty {
                    AssetOtherListings(listings: detail.otherListings) {
                        environment.navigator.open(AssetRoute(symbol: $0.symbol), in: tab)
                    }
                }
            }
            .padding(.bottom, MonacoTheme.Space.l)
        }
        .monacoCanvas()
        .foregroundStyle(MonacoTheme.primaryText)
        .monacoFrameStats("AssetDetail")
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("asset-detail-root")
    }

    private var chartStyle: Binding<MonacoChartStyle> {
        Binding(
            get: { MonacoChartStyle(rawValue: storedStyle) ?? .candles },
            set: { storedStyle = $0.rawValue }
        )
    }

    private func toggleWatch() {
        Haptics.selection()
        if environment.watchlist.toggle(symbol) == .full {
            toasts.current = MonacoToast(message: WatchlistCopy.fullMessage)
        }
    }

    private func propose(_ kind: ProposeKind) {
        AssetDetailBuyCTA.open(symbol: symbol, kind: kind, tab: tab) { environment.navigator.open($0, in: $1) }
    }

    private func start() async {
        if model == nil {
            model = AssetDetailClientModel(api: environment.api, symbol: symbol, hints: environment.hints)
        }
        await model?.start()
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
