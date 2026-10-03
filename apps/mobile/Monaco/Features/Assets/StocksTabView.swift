import MonacoAPI
import MonacoCore
import SwiftUI

typealias CatalogStocksTabModel = MonacoCore.StocksTabModel
struct StocksTabView: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @State private var model: CatalogStocksTabModel?
    init(model: CatalogStocksTabModel? = nil) {
        _model = State(initialValue: model)
    }
    var body: some View {
        Group {
            if let model {
                StocksTabScreen(model: model, open: open)
            }
        }
        .navigationTitle(StocksTab.title)
        .navigationBarTitleDisplayMode(.large)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("assets-root")
        .task { await start() }
        .onScreenVisibilityChange { visible in
            model?.setVisible(visible)
        }
        .onChange(of: model?.failureTick) { _, _ in
            guard let error = model?.lastError else { return }
            toasts.show(error)
        }
    }
    private func start() async {
        let model = preparedModel()
        if model.rows.isEmpty { await model.load() }
        await model.observe()
    }
    private func preparedModel() -> CatalogStocksTabModel {
        if let model { return model }
        let created = CatalogStocksTabModel(api: environment.api, hints: environment.hints, clock: ContinuousClock())
        model = created
        return created
    }
    private func open(_ asset: MarketAsset) {
        Haptics.selection()
        environment.navigator.open(AssetRoute(symbol: asset.symbol), in: .stocks)
    }
}
private struct StocksTabScreen: View {
    let model: CatalogStocksTabModel
    let open: (MarketAsset) -> Void
    @State private var query = ""
    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            searchField
            if !model.isSearching {
                StocksFilterBar(browse: model.browse) { browse in
                    Task { await model.show(browse) }
                }
            }
            list
        }
        .padding(.vertical, MonacoTheme.Space.m)
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
        .monacoCanvas()
        .foregroundStyle(MonacoTheme.ink)
        .onChange(of: query) { _, newValue in
            model.setQuery(newValue)
        }
    }
    private var searchField: some View {
        MonacoSearchField(placeholder: "Search Apple, Tesla, NVDA…", text: $query)
            .padding(.horizontal, MonacoTheme.Space.m)
    }
    @ViewBuilder
    private var list: some View {
        switch model.phase {
        case .idle,
            .loading where model.rows.isEmpty:
            ProgressView()
                .frame(maxWidth: .infinity, maxHeight: .infinity)
                .accessibilityIdentifier("assets-popular-loading")
        case .failed where model.rows.isEmpty:
            EmptyState(title: emptyFailureTitle, actionTitle: "Retry") {
                Task { await model.load() }
            }
            .accessibilityIdentifier("assets-popular-failed")
        default:
            stocks
        }
    }
    private var stocks: some View {
        ScrollView {
            if model.rows.isEmpty {
                EmptyState(title: emptyTitle)
                    .accessibilityIdentifier(model.isSearching ? "assets-search-empty" : "assets-popular-empty")
            } else {
                sectionTitle
                StocksAssetList(rows: model.rows, open: open)
                if model.loadMoreFailed {
                    Text("Could not load more stocks.")
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.muted)
                        .frame(maxWidth: .infinity)
                        .padding(.top, MonacoTheme.Space.s)
                }
                if model.hasMore {
                    Button(model.isLoadingMore ? "Loading…" : "Load more") {
                        Task { await model.loadMore() }
                    }
                    .buttonStyle(.monacoSecondary)
                    .disabled(model.isLoadingMore)
                    .padding(.top, MonacoTheme.Space.s)
                    .accessibilityIdentifier("assets-load-more")
                }
            }
        }
        .refreshable { await model.load() }
        .accessibilityIdentifier(model.isSearching ? "assets-grid-search" : "assets-grid-popular")
    }
    @ViewBuilder
    private var sectionTitle: some View {
        if !model.isSearching {
            MonacoSectionHeader(model.browse == .preIpo ? PreIpoCopy.sectionTitle : "Popular")
                .padding(.horizontal, MonacoTheme.Space.m)
                .accessibilityIdentifier(model.browse == .preIpo ? "assets-preipo" : "assets-popular")
        }
    }
    private var emptyTitle: String {
        model.isSearching ? "No matches for that search" : "No stocks to show"
    }
    private var emptyFailureTitle: String {
        model.isSearching ? "Could not load stocks" : "Could not load popular stocks"
    }
}
private struct StocksFilterBar: View {
    let browse: CatalogStocksTabModel.Browse
    let select: (CatalogStocksTabModel.Browse) -> Void
    var body: some View {
        HStack(spacing: MonacoTheme.Space.s) {
            chip("Popular", .popular, "assets-filter-popular")
            chip(PreIpoCopy.sectionTitle, .preIpo, "assets-filter-preipo")
            Spacer(minLength: 0)
        }
        .padding(.horizontal, MonacoTheme.Space.m)
    }
    private func chip(_ title: String, _ browse: CatalogStocksTabModel.Browse, _ identifier: String) -> some View {
        Button(title) { select(browse) }
            .buttonStyle(.plain)
            .font(self.browse == browse ? MonacoTheme.Typo.bodyStrong : MonacoTheme.Typo.body)
            .foregroundStyle(self.browse == browse ? MonacoTheme.ink : MonacoTheme.muted)
            .frame(minHeight: 44)
            .accessibilityIdentifier(identifier)
            .accessibilityAddTraits(self.browse == browse ? .isSelected : [])
    }
}
private struct StocksAssetList: View {
    let rows: [MarketAsset]
    let open: (MarketAsset) -> Void
    var body: some View {
        MonacoGroupedList {
            ForEach(rows) { asset in
                Button {
                    open(asset)
                } label: {
                    StocksAssetRow(asset: asset, isLast: asset.id == rows.last?.id)
                }
                .buttonStyle(.monacoRow)
                .accessibilityIdentifier("assets-row-\(asset.symbol)")
            }
        }
    }
}
private struct StocksAssetRow: View {
    let asset: MarketAsset
    var isLast = false
    var body: some View {
        ViewThatFits(in: .horizontal) {
            wide
            stacked
        }
        .padding(.horizontal, MonacoTheme.Space.m)
        .padding(.vertical, MonacoTheme.Space.s)
        .frame(minHeight: 64)
        .overlay(alignment: .bottom) { hairline }
        .accessibilityElement(children: .combine)
    }
    private var wide: some View {
        HStack(spacing: MonacoTheme.Space.sm) {
            mark
            names
            spark
            figures
        }
    }
    private var stacked: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            HStack(spacing: MonacoTheme.Space.sm) {
                mark
                names
            }
            figures
        }
    }
    private var mark: some View {
        StockMark(
            symbol: asset.symbol,
            displayName: asset.name,
            assetKind: asset.kind,
            size: 46,
            logoURL: asset.logoURL
        )
        .frame(width: 46, height: 46)
    }
    private var names: some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(asset.name)
                .font(MonacoTheme.Typo.bodyStrong)
                .foregroundStyle(MonacoTheme.ink)
                .lineLimit(2)
            Text(asset.ticker)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
                .lineLimit(1)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
    @ViewBuilder
    private var spark: some View {
        if let series = asset.sparkline {
            Sparkline(series: series, tone: tone)
                .accessibilityHidden(true)
        }
    }
    private var figures: some View {
        VStack(alignment: .trailing, spacing: 3) {
            price
            change
        }
    }
    private var price: some View {
        HStack(spacing: 4) {
            if asset.status.afterHours {
                Image(systemName: "moon.fill")
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.warning)
                    .accessibilityLabel("After hours")
            }
            if let micros = asset.priceMicros {
                MoneyText(micros: micros, style: .row, voice: .market)
            } else {
                Text("—")
                    .font(MonacoTheme.Typo.bodyStrong)
                    .foregroundStyle(MonacoTheme.muted)
            }
        }
    }
    @ViewBuilder
    private var change: some View {
        if let basisPoints = asset.changeBasisPoints {
            PercentText(basisPoints: basisPoints, style: .caption)
        } else {
            Text("—")
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
        }
    }
    @ViewBuilder
    private var hairline: some View {
        if !isLast {
            Rectangle()
                .fill(MonacoTheme.hairline)
                .frame(height: 1)
                .padding(.leading, 46 + MonacoTheme.Space.sm + MonacoTheme.Space.m)
        }
    }
    private var tone: PnLTone {
        guard let basisPoints = asset.changeBasisPoints, basisPoints != 0 else { return .flat }
        return basisPoints < 0 ? .loss : .profit
    }
}
