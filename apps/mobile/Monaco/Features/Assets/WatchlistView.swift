import MonacoAPI
import MonacoCore
import SwiftUI

nonisolated struct WatchlistRoute: AppRoute {
    var query = ""

    @MainActor func destination() -> some View {
        WatchlistView(query: query)
    }
}

struct WatchlistView: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(\.hostMainTab) private var hostMainTab
    @State private var model: MonacoCore.StocksTabModel?
    @State private var query: String
    private let makeModel: @MainActor (AppEnvironment) -> MonacoCore.StocksTabModel

    init(
        query: String = "",
        makeModel: @escaping @MainActor (AppEnvironment) -> MonacoCore.StocksTabModel = WatchlistView.liveModel
    ) {
        _query = State(initialValue: query)
        self.makeModel = makeModel
    }

    var body: some View {
        ZStack {
            if let model {
                WatchlistContent(
                    model: model, watchlist: environment.watchlist, query: $query, open: open, toggle: toggle)
            } else {
                WatchlistSkeleton(query: $query)
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
        .monacoCanvas()
        .foregroundStyle(MonacoTheme.primaryText)
        .navigationTitle("Your watchlist")
        .navigationBarTitleDisplayMode(.inline)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("watchlist-root")
        .task { await start() }
        .onScreenVisibilityChange { model?.setVisible($0) }
        .onChange(of: query) { _, value in model?.setQuery(value) }
        .onChange(of: model?.failureTick) { _, _ in
            guard let error = model?.lastError else { return }
            toasts.show(error)
        }
    }

    static func liveModel(_ environment: AppEnvironment) -> MonacoCore.StocksTabModel {
        MonacoCore.StocksTabModel(api: environment.api, hints: environment.hints, clock: ContinuousClock())
    }

    private func start() async {
        let created = model ?? makeModel(environment)
        model = created
        if !query.isEmpty {
            created.setQuery(query)
        } else if created.rows.isEmpty {
            await created.load()
        }
        await created.observe()
    }

    private func open(_ asset: MarketAsset) {
        Haptics.selection()
        environment.navigator.open(AssetRoute(symbol: asset.symbol), in: hostMainTab ?? .stocks)
    }

    private func toggle(_ asset: MarketAsset) {
        Haptics.selection()
        if environment.watchlist.toggle(asset.symbol) == .full {
            toasts.current = MonacoToast(message: WatchlistCopy.fullMessage)
        }
    }
}

private struct WatchlistContent: View {
    let model: MonacoCore.StocksTabModel
    let watchlist: WatchlistModel
    @Binding var query: String
    let open: (MarketAsset) -> Void
    let toggle: (MarketAsset) -> Void

    var body: some View {
        if model.rows.isEmpty, !model.isSearching, case .failed = model.phase {
            AssetsLoadFailure(identifier: "watchlist-failed") { Task { await model.load() } }
        } else {
            ScrollView {
                LazyVStack(alignment: .leading, spacing: 0) {
                    WatchlistHeader(query: $query, count: watchlist.symbols.count)
                    results
                }
            }
            .scrollDismissesKeyboard(.interactively)
            .refreshable { await model.load() }
        }
    }

    @ViewBuilder private var results: some View {
        if model.rows.isEmpty {
            switch model.phase {
            case .idle, .loading:
                MonacoRowSkeleton(rows: 4, hasTrailing: false)
            case .failed:
                MonacoErrorRow(thing: "stocks", identifier: "watchlist-search-failed") {
                    Task { await model.load() }
                }
            case .loaded:
                EmptyState(
                    title: "No matching stocks", message: "Try another search.", systemImage: "magnifyingglass"
                )
                .accessibilityIdentifier("watchlist-search-empty")
            }
        } else {
            if watchlist.symbols.isEmpty, !model.isSearching {
                EmptyState(
                    title: "Your watchlist is empty", message: "Add stocks using the stars below.",
                    systemImage: "star"
                )
                .accessibilityIdentifier("watchlist-empty")
            }
            rows
        }
    }

    private var rows: some View {
        ForEach(model.rows) { asset in
            WatchlistRow(
                asset: asset, isWatched: watchlist.isWatched(asset.symbol), isLast: asset.id == model.rows.last?.id,
                open: { open(asset) }, toggle: { toggle(asset) }
            )
            .onAppear {
                guard model.hasMore, asset.id == model.rows.last?.id else { return }
                Task { await model.loadMore() }
            }
        }
    }
}

private struct WatchlistHeader: View {
    @Binding var query: String
    let count: Int

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            MonacoSearchField(placeholder: "Find stocks for your watchlist", text: $query)
                .padding(.top, MonacoTheme.Space.m)
            Text(WatchlistCopy.countLine(count))
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
                .padding(.top, MonacoTheme.Space.s)
                .padding(.bottom, MonacoTheme.Space.sm)
                .accessibilityIdentifier("watchlist-count")
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
    }
}

private struct WatchlistSkeleton: View {
    @Binding var query: String

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            WatchlistHeader(query: $query, count: 0)
            MonacoRowSkeleton(rows: 4, hasTrailing: false)
        }
    }
}

private struct WatchlistRow: View {
    let asset: MarketAsset
    let isWatched: Bool
    let isLast: Bool
    let open: () -> Void
    let toggle: () -> Void

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    private var name: String { AssetDisplayName.format(catalogName: asset.name, kind: asset.kind) }

    var body: some View {
        HStack(spacing: MonacoTheme.Space.sm) {
            Button(action: open) {
                HStack(spacing: MonacoTheme.Space.sm) {
                    StockMark(
                        symbol: asset.symbol, displayName: asset.name, assetKind: asset.kind, size: .row,
                        logoURL: asset.logoURL)
                    labels
                    Spacer(minLength: 0)
                }
                .frame(minHeight: MonacoButtonMetrics.hitTarget)
                .contentShape(Rectangle())
            }
            .buttonStyle(.monacoRow)
            .accessibilityLabel("\(name), \(asset.ticker)")
            .accessibilityIdentifier("watchlist-row-\(asset.symbol)")
            WatchStarButton(
                isWatched: isWatched, addLabel: "Add \(name)", removeLabel: "Remove \(name)",
                identifier: "watchlist-star-\(asset.symbol)", toggle: toggle)
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.vertical, MonacoTheme.Space.sm)
        .frame(minHeight: MonacoRowLayout.minHeight)
        .overlay(alignment: .bottom) {
            if !isLast { MonacoRule().padding(.horizontal, MonacoTheme.Space.gutter) }
        }
    }

    private var labels: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
            Text(name)
                .font(MonacoTheme.Typo.headline)
                .foregroundStyle(MonacoTheme.primaryText)
                .lineLimit(dynamicTypeSize.isAccessibilitySize ? 2 : 1)
            Text(asset.ticker)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
                .lineLimit(1)
        }
    }
}
