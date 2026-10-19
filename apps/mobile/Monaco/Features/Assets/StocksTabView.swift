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
                    model: model, query: $query, open: { openSymbol($0.symbol) }, openSymbol: openSymbol,
                    openWatchlist: { environment.navigator.open(WatchlistRoute(), in: .stocks) })
            } else {
                ScrollView { MarketsLoading() }
            }
        }
        .monacoTopLevelHeader(title: StocksTab.title)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("assets-root")
        .task { await start() }
        .onScreenVisibilityChange { model?.setVisible($0) }
        .onChange(of: model?.failureTick) { _, _ in
            guard let error = model?.lastError else { return }
            toasts.show(error)
        }
        .onChange(of: environment.watchlist.symbols) { _, _ in
            Task { await model?.quotes?.load() }
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

    private enum Stage {
        case loading, failed, empty, browse, search
    }

    private var stage: Stage {
        if model.isSearching { return .search }
        guard model.browse == .all, model.rows.isEmpty else { return .browse }
        switch model.phase {
        case .idle, .loading: return .loading
        case .failed: return .failed
        case .loaded: return .empty
        }
    }

    private var popularIsLoading: Bool {
        guard model.popular.rows.isEmpty else { return false }
        switch model.popular.phase {
        case .idle, .loading: return true
        case .loaded, .failed: return false
        }
    }

    var body: some View {
        content
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
            .monacoCanvas()
            .foregroundStyle(MonacoTheme.primaryText)
            .onChange(of: query) { _, value in model.setQuery(value) }
    }

    @ViewBuilder private var content: some View {
        switch stage {
        case .loading:
            ScrollView { MarketsLoading() }
        case .failed:
            AssetsLoadFailure(identifier: "assets-failed") { Task { await model.loadMarkets() } }
        case .empty, .browse, .search:
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
            MonacoSearchField(placeholder: "Search stocks or tickers", text: $query)
            if let minutes = model.staleMinutes {
                MonacoNotice(.warn, message: MarketStaleness.pricesDelayed(minutes: minutes))
                    .accessibilityIdentifier("markets-stale")
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
    }

    @ViewBuilder private var sections: some View {
        switch stage {
        case .search:
            MarketsAllStocks(model: model, title: "Search results", open: open)
        case .empty:
            EmptyState(title: "No stocks to show", systemImage: "chart.line.uptrend.xyaxis")
                .accessibilityIdentifier("assets-empty")
        case .browse:
            if popularIsLoading || !model.topMovers.isEmpty {
                MarketsMoversSection(movers: model.topMovers, isLoading: popularIsLoading, open: open)
            }
            MarketsWatchlistSection(rows: model.quotes?.rows ?? [], open: openSymbol, manage: openWatchlist)
            if popularIsLoading || !model.popularNow.isEmpty {
                MarketsPopularSection(rows: model.popularNow, isLoading: popularIsLoading, open: open)
            }
            MarketsAllStocks(model: model, title: "All stocks", open: open)
        case .loading, .failed:
            EmptyView()
        }
    }
}
