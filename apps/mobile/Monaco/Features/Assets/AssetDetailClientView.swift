import MonacoAPI
import MonacoCore
import SwiftUI

struct AssetDetailClientView: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(\.hostMainTab) private var hostMainTab
    let symbol: String
    let seed: AssetSeed?
    @State private var model: AssetDetailClientModel?
    @State private var scrubbedIndex: Int?
    @State private var priceTick: MonacoPriceTick?
    @State private var showsStatsInfo = false

    init(symbol: String, seed: AssetSeed? = nil, model: AssetDetailClientModel? = nil) {
        self.symbol = symbol
        self.seed = seed
        _model = State(initialValue: model)
    }

    private var tab: MainTab { hostMainTab ?? .stocks }

    var body: some View {
        content
            .navigationBarTitleDisplayMode(.inline)
            .toolbar { toolbar }
            .sheet(isPresented: $showsStatsInfo) {
                if let facts = model?.detail?.marketStats { AssetMarketStatsInfo(facts: facts) }
            }
            .task {
                await start()
                await model?.observe()
            }
            .onScreenVisibilityChange { visible in model?.setVisible(visible) }
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
                if seed?.kind == .preIpo { preIpoPage(.loading, model: model) } else { loading }
            case .failed:
                if seed?.kind == .preIpo {
                    preIpoPage(.failed, model: model)
                } else {
                    AssetsLoadFailure(identifier: "asset-detail-failed") { Task { await model.start() } }
                        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
                        .monacoCanvas()
                }
            case .loaded:
                if let detail = model.detail {
                    if detail.kind == .preIpo {
                        preIpoPage(.loaded(detail), model: model)
                    } else {
                        loaded(detail, model: model)
                    }
                }
            }
        } else {
            loading
        }
    }

    private func preIpoPage(_ state: PreIpoPageState, model: AssetDetailClientModel) -> some View {
        let detail = model.detail
        return PreIpoDetailView(
            symbol: symbol, name: detail?.name ?? seed?.name ?? symbol,
            logoURL: detail?.market.logoURL ?? seed?.logoURL,
            state: state, held: model.heldRows, proposals: model.openProposals,
            openCabal: { environment.navigator.open(CabalRoute(id: $0), in: tab) },
            openProposal: { environment.navigator.open(ProposalRoute(proposalID: $0), in: tab) },
            propose: propose, retry: { Task { await model.start() } })
    }

    private var loading: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                if let seed {
                    AssetDetailSeedHeader(symbol: symbol, seed: seed)
                    SkeletonCardBlock()
                } else {
                    AssetDetailHeroSkeleton()
                }
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
                    scrub: AssetScrubHeader(chart: model.chart, barIndex: scrubbedIndex), priceTick: priceTick)
                AssetDetailChartSection(detail: detail, model: model, scrubbedIndex: $scrubbedIndex)
                AssetDetailActions(detail: detail, canSell: !model.heldRows.isEmpty, propose: propose)
                if AssetAbout.isShown(kind: detail.kind) { AssetAbout(detail: detail) }
                AssetInYourCabals(
                    held: model.heldRows, proposals: model.openProposals,
                    openCabal: { environment.navigator.open(CabalRoute(id: $0), in: tab) },
                    openProposal: { environment.navigator.open(ProposalRoute(proposalID: $0), in: tab) })
                if let stats = model.stats {
                    AssetMarketStats(stats: stats) { showsStatsInfo = true }
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

    @ToolbarContentBuilder private var toolbar: some ToolbarContent {
        if model?.detail != nil {
            ToolbarItem(placement: .topBarTrailing) {
                let isWatched = environment.watchlist.isWatched(symbol)
                MonacoGlassButton(
                    systemImage: "star", accessibilityLabel: isWatched ? "Remove from watchlist" : "Add to watchlist",
                    style: isWatched ? .selected : .quiet, placement: .toolbar, action: toggleWatch
                )
                .accessibilityAddTraits(isWatched ? .isSelected : [])
                .accessibilityIdentifier("asset-watch-star")
            }
            ToolbarItem(placement: .topBarTrailing) {
                MonacoGlassButton(
                    icon: .share, accessibilityLabel: "Share \(model?.detail?.name ?? symbol)", style: .quiet,
                    placement: .toolbar, action: share
                )
                .accessibilityIdentifier("asset-share")
            }
        }
    }

    private func toggleWatch() {
        Haptics.selection()
        switch environment.watchlist.toggle(symbol) {
        case .full: toasts.current = MonacoToast(message: WatchlistCopy.fullMessage)
        case .added: toasts.current = MonacoToast(message: "Added to your watchlist")
        case .removed: break
        }
    }

    private func share() {
        if let held = model?.heldRows.first {
            environment.navigator.open(ShareRoute(.position(cabalID: held.cabalID, symbol: symbol)), in: tab)
        } else {
            let name = model?.detail?.name ?? symbol
            SystemShare.present(SharePayload(title: "\(name) on Monaco", url: SharePayload.fallbackURL))
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
