import MonacoAPI
import MonacoCore
import SwiftUI

struct StocksTabView: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @State private var model: MonacoCore.StocksTabModel?
    @State private var query: String
    private let makeModel: @MainActor (AppEnvironment) -> MonacoCore.StocksTabModel

    init(
        query: String = "",
        makeModel: @escaping @MainActor (AppEnvironment) -> MonacoCore.StocksTabModel = StocksTabView.liveModel
    ) {
        _query = State(initialValue: query)
        self.makeModel = makeModel
    }

    var body: some View {
        ZStack {
            if let model {
                StocksTabScreen(
                    model: model, query: $query, open: openAsset, openSymbol: openSymbol,
                    openWatchlist: { environment.navigator.open(WatchlistRoute(), in: .stocks) })
            } else {
                ScrollView { MarketsLoading() }
            }
        }
        .monacoTopLevelHeader(title: StocksTab.title)
        .toolbar {
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
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("assets-root")
        .task { await start() }
        .onScreenVisibilityChange { model?.setVisible($0) }
        .onChange(of: model?.failureTick) { _, _ in
            guard let error = model?.lastError else { return }
            toasts.show(error)
        }
        .onChange(of: environment.watchlist.symbols) { _, _ in
            Task { await model?.watchlistChanged() }
        }
    }

    private func start() async {
        let created = preparedModel()
        if !query.isEmpty {
            created.setQuery(query)
        } else if created.rows.isEmpty {
            await created.loadMarkets()
        }
        await created.observe()
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
    @Binding var query: String
    let open: (MarketAsset) -> Void
    let openSymbol: (String) -> Void
    let openWatchlist: () -> Void

    @State private var showsPreIpoInfo = false

    private enum Stage {
        case loading, failed, browse, search
    }

    private var stage: Stage {
        if model.isSearching { return .search }
        guard model.preIpo.rows.isEmpty, model.watchlistRows.isEmpty else { return .browse }
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

    @ViewBuilder private var content: some View {
        switch stage {
        case .loading:
            ScrollView { MarketsLoading() }
        case .failed:
            AssetsLoadFailure(identifier: "assets-failed") { Task { await model.loadMarkets() } }
        case .browse, .search:
            ScrollView {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.section) {
                    header
                    sections
                }
                .padding(.top, MonacoTheme.Space.s)
                .padding(.bottom, MonacoTheme.Space.l)
            }
            .scrollDismissesKeyboard(.interactively)
            .refreshable { await model.loadMarkets() }
            .accessibilityIdentifier(model.isSearching ? "assets-grid-search" : "assets-grid")
        }
    }

    private var header: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            MonacoSearchField(placeholder: "Stocks and Pre-IPO", text: $query)
                .padding(.horizontal, MonacoTheme.Space.gutter)
            if !model.isSearching {
                MarketsChips(selected: model.browse) { next in Task { await model.show(next) } }
            }
            if let minutes = model.staleMinutes {
                MonacoNotice(.warn, message: MarketStaleness.pricesDelayed(minutes: minutes))
                    .accessibilityIdentifier("markets-stale")
                    .padding(.horizontal, MonacoTheme.Space.gutter)
            }
        }
    }

    @ViewBuilder private var sections: some View {
        if stage == .search {
            MarketsSearchResults(model: model, open: open)
        } else {
            switch model.browse {
            case .all: allSections
            case .watchlist: watchlistChip
            case .preIpo: preIpoChip
            case .held: EmptyView()
            }
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
