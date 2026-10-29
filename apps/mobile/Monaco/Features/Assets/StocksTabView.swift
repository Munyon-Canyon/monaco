import MonacoAPI
import MonacoCore
import SwiftUI

struct StocksTabView: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @State private var model: MonacoCore.StocksTabModel?
    @State private var held: HeldModel?
    @State private var screenVisible = true
    @State private var query: String
    @State private var searching: Bool
    private let makeModel: @MainActor (AppEnvironment) -> MonacoCore.StocksTabModel
    private let makeHeld: @MainActor (AppEnvironment) -> HeldModel

    #if DEBUG
    @MainActor static var sampleBrowse = MonacoCore.StocksTabModel.Browse.all
    #endif

    init(
        query: String = "",
        makeModel: @escaping @MainActor (AppEnvironment) -> MonacoCore.StocksTabModel = StocksTabView.liveModel,
        makeHeld: @escaping @MainActor (AppEnvironment) -> HeldModel = StocksTabView.liveHeld
    ) {
        _query = State(initialValue: query)
        _searching = State(initialValue: !query.isEmpty)
        self.makeModel = makeModel
        self.makeHeld = makeHeld
    }

    var body: some View {
        ZStack {
            if let model, let held {
                StocksTabScreen(
                    model: model, held: held, query: $query, searching: searching, open: openAsset,
                    openSymbol: openSymbol,
                    openWatchlist: { environment.navigator.open(WatchlistRoute(), in: .stocks) },
                    openMovers: { environment.navigator.open(MarketsListRoute(order: .movers), in: .stocks) })
            } else {
                ScrollView { MarketsLoading() }
            }
        }
        .monacoTopLevelHeader(title: StocksTab.title)
        .searchable(
            text: $query, isPresented: $searching, placement: .navigationBarDrawer(displayMode: .always),
            prompt: "Stocks and Pre-IPO"
        )
        .toolbar {
            if !searching {
                ToolbarItem(placement: .topBarTrailing) {
                    MonacoGlassButton(
                        systemImage: MonacoIcon.star.symbol, accessibilityLabel: "Your watchlist", style: .quiet,
                        placement: .toolbar
                    ) {
                        environment.navigator.open(WatchlistRoute(), in: .stocks)
                    }
                    .accessibilityIdentifier("markets-watchlist-manage")
                }
                .withoutGlass()
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("assets-root")
        .task { await start() }
        .onScreenVisibilityChange {
            model?.setVisible($0)
            screenVisible = $0
            syncHeldVisibility()
        }
        .onChange(of: model?.failureTick) { _, _ in
            guard let error = model?.lastError else { return }
            toasts.show(error)
        }
        .onChange(of: held?.failureTick) { _, _ in
            guard let error = held?.lastError else { return }
            toasts.show(error)
        }
        .onChange(of: model?.browse) { _, next in
            guard next == .held else { return syncHeldVisibility() }
            Task {
                await held?.load()
                syncHeldVisibility()
            }
        }
        .onChange(of: environment.watchlist.symbols) { _, _ in
            Task { await model?.watchlistChanged() }
        }
    }

    private func start() async {
        let created = preparedModel()
        let heldModel = preparedHeld()
        if !query.isEmpty {
            created.setQuery(query)
        } else if created.rows.isEmpty {
            await created.loadMarkets()
        }
        #if DEBUG
        if Self.sampleBrowse != .all { await created.show(Self.sampleBrowse) }
        #endif
        async let heldWatch: Void = heldModel.observe()
        await created.observe()
        await heldWatch
    }

    private func syncHeldVisibility() {
        held?.setVisible(screenVisible && model?.browse == .held)
    }

    private func preparedHeld() -> HeldModel {
        if let held { return held }
        let created = makeHeld(environment)
        created.setVisible(false)
        held = created
        return created
    }

    private func preparedModel() -> MonacoCore.StocksTabModel {
        if let model { return model }
        let created = makeModel(environment)
        model = created
        return created
    }

    static func liveModel(_ environment: AppEnvironment) -> MonacoCore.StocksTabModel {
        MonacoCore.StocksTabModel(
            api: environment.api, hints: environment.hints, clock: ContinuousClock(),
            watchlist: environment.watchlist)
    }

    static func liveHeld(_ environment: AppEnvironment) -> HeldModel {
        HeldModel(api: environment.api, hints: environment.hints, clock: ContinuousClock())
    }

    private func openAsset(_ asset: MarketAsset) {
        Haptics.selection()
        environment.navigator.open(AssetRoute(symbol: asset.symbol, seed: AssetSeed(asset)), in: .stocks)
    }

    private func openSymbol(_ symbol: String) {
        Haptics.selection()
        environment.navigator.open(AssetRoute(symbol: symbol), in: .stocks)
    }
}

private struct StocksTabScreen: View {
    let model: MonacoCore.StocksTabModel
    let held: HeldModel
    @Binding var query: String
    let searching: Bool
    let open: (MarketAsset) -> Void
    let openSymbol: (String) -> Void
    let openWatchlist: () -> Void
    let openMovers: () -> Void

    @State private var showsPreIpoInfo = false

    private enum Stage {
        case loading, failed, browse
    }

    private var stage: Stage {
        guard model.browse != .held, model.preIpo.rows.isEmpty, model.watchlistRows.isEmpty else { return .browse }
        switch model.preIpo.phase {
        case .idle, .loading: return .loading
        case .failed: return .failed
        case .loaded: return .browse
        }
    }

    var body: some View {
        content
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
            .monacoCanvas()
            .foregroundStyle(MonacoTheme.primaryText)
            .onChange(of: query) { _, value in model.setQuery(value) }
            .sheet(isPresented: $showsPreIpoInfo) { MarketsPreIpoInfoSheet() }
    }

    private var content: some View {
        ZStack {
            browseContent
                .opacity(searching ? 0 : 1)
                .allowsHitTesting(!searching)
                .accessibilityHidden(searching)
            if searching {
                MarketsSearch(model: model, open: open)
                    .background(MonacoTheme.canvas)
            }
        }
    }

    @ViewBuilder private var browseContent: some View {
        switch stage {
        case .loading:
            ScrollView { MarketsLoading() }
        case .failed:
            AssetsLoadFailure(identifier: "assets-failed") { Task { await model.loadMarkets() } }
        case .browse:
            ScrollView {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.section) {
                    header
                    sections
                }
                .padding(.top, MonacoTheme.Space.s)
                .padding(.bottom, MonacoTheme.Space.homeClear)
            }
            .scrollDismissesKeyboard(.interactively)
            .refreshable {
                if model.browse == .held {
                    await held.load()
                } else {
                    await model.loadMarkets()
                }
            }
            .accessibilityIdentifier("assets-grid")
        }
    }

    private var header: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            MarketsChips(selected: model.browse) { next in Task { await model.show(next) } }
            if let minutes = model.staleMinutes {
                MonacoNotice(.warn, message: MarketStaleness.pricesDelayed(minutes: minutes))
                    .accessibilityIdentifier("markets-stale")
                    .padding(.horizontal, MonacoTheme.Space.gutter)
            }
        }
    }

    @ViewBuilder private var sections: some View {
        switch model.browse {
        case .all: allSections
        case .watchlist: watchlistChip
        case .preIpo: preIpoChip
        case .held: MarketsHeldList(model: held, openSymbol: openSymbol)
        }
    }

    @ViewBuilder private var allSections: some View {
        if model.watchlistRows.isEmpty, model.preIpo.phase == .loaded, model.preIpo.rows.isEmpty {
            EmptyState(title: "No stocks to show", systemImage: "chart.line.uptrend.xyaxis")
                .accessibilityIdentifier("assets-empty")
        }
        MarketsWatchlistSection(
            model: model, rows: model.watchlistPreview, open: open, openSymbol: openSymbol,
            seeAll: { Task { await model.show(.watchlist) } }, manage: openWatchlist)
        if !model.preIpoPreview.isEmpty {
            MarketsPreIpoSection(
                rows: model.preIpoPreview, open: open, seeAll: { Task { await model.show(.preIpo) } },
                info: { showsPreIpoInfo = true })
        }
        if !model.moverPreview.isEmpty {
            MarketsMovingToday(movers: model.moverPreview, open: open, seeAll: openMovers)
        }
    }

    private var watchlistChip: some View {
        MarketsWatchlistSection(
            model: model, rows: model.watchlistRows, showsSeeAll: false, open: open, openSymbol: openSymbol,
            seeAll: {}, manage: openWatchlist)
    }

    @ViewBuilder private var preIpoChip: some View {
        if model.preIpo.rows.isEmpty {
            EmptyState(title: "No Pre-IPO companies yet", systemImage: "chart.line.uptrend.xyaxis")
                .accessibilityIdentifier("assets-empty")
        } else {
            MarketsPreIpoSection(
                rows: model.preIpo.rows, showsSeeAll: false, open: open, seeAll: {},
                info: { showsPreIpoInfo = true },
                onLastAppear: {
                    guard model.hasMore else { return }
                    Task { await model.loadMore() }
                })
        }
    }
}

extension ToolbarContent {
    @ToolbarContentBuilder
    fileprivate func withoutGlass() -> some ToolbarContent {
        if #available(iOS 26.0, *) {
            sharedBackgroundVisibility(.hidden)
        } else {
            self
        }
    }
}
