import MonacoAPI
import MonacoCore
import SwiftUI

struct StocksTabView: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @State private var model: MonacoCore.StocksTabModel?
    private let makeModel: @MainActor (AppEnvironment) -> MonacoCore.StocksTabModel

    init(makeModel: @escaping @MainActor (AppEnvironment) -> MonacoCore.StocksTabModel = StocksTabView.liveModel) {
        self.makeModel = makeModel
    }

    var body: some View {
        ZStack {
            if let model {
                StocksTabScreen(model: model, open: open)
            } else {
                StockRowSkeleton().frame(maxHeight: .infinity, alignment: .top)
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
    }

    private func start() async {
        let model = preparedModel()
        if model.rows.isEmpty { await model.load() }
        await model.observe()
    }

    private func preparedModel() -> MonacoCore.StocksTabModel {
        if let model { return model }
        let created = makeModel(environment)
        model = created
        return created
    }

    static func liveModel(_ environment: AppEnvironment) -> MonacoCore.StocksTabModel {
        MonacoCore.StocksTabModel(api: environment.api, hints: environment.hints, clock: ContinuousClock())
    }

    private func open(_ asset: MarketAsset) {
        Haptics.selection()
        environment.navigator.open(AssetRoute(symbol: asset.symbol), in: .stocks)
    }
}

private struct StocksTabScreen: View {
    let model: MonacoCore.StocksTabModel
    let open: (MarketAsset) -> Void
    @State private var query = ""

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            MonacoSearchField(placeholder: "Search Apple, Tesla, NVDA…", text: $query)
                .padding(.horizontal, MonacoTheme.Space.gutter)
            MonacoChipBar(
                items: MonacoCore.StocksTabModel.Browse.allCases, selected: model.browse, title: \.title,
                identifierPrefix: "stocks-chip"
            ) { browse in
                Task { await model.show(browse) }
            }
            ScrollView { content }
                .scrollDismissesKeyboard(.interactively)
                .refreshable { await model.load() }
                .accessibilityIdentifier(model.isSearching ? "assets-grid-search" : "assets-grid")
        }
        .padding(.vertical, MonacoTheme.Space.m)
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
        .monacoCanvas()
        .foregroundStyle(MonacoTheme.ink)
        .onChange(of: query) { _, value in model.setQuery(value) }
    }

    @ViewBuilder private var content: some View {
        if !model.rows.isEmpty {
            rows(model.rows, loadsMore: model.hasMore)
        } else {
            switch model.phase {
            case .idle, .loading:
                StockRowSkeleton()
            case .failed:
                MonacoErrorRow(thing: "stocks", identifier: "assets-failed") { Task { await model.load() } }
            case .loaded:
                if model.isSearching {
                    EmptyState(title: searchEmptyTitle)
                        .accessibilityIdentifier("assets-search-empty")
                } else {
                    EmptyState(title: "No stocks to show")
                }
            }
        }
    }

    private var searchEmptyTitle: String {
        guard model.browse != .all else { return "No stocks match “\(query)”" }
        return "No \(model.browse.title) stocks match “\(query)”"
    }

    private func rows(_ assets: [MarketAsset], loadsMore: Bool = false) -> some View {
        VStack(spacing: MonacoTheme.Space.s) {
            LazyVStack(spacing: 0) {
                ForEach(assets) { asset in
                    Button {
                        open(asset)
                    } label: {
                        StocksAssetRow(asset: asset, isLast: asset.id == assets.last?.id)
                    }
                    .buttonStyle(.monacoRow)
                    .accessibilityIdentifier("assets-row-\(asset.symbol)")
                    .onAppear {
                        guard loadsMore, asset.id == assets.last?.id else { return }
                        Task { await model.loadMore() }
                    }
                }
            }
            .frame(maxWidth: .infinity)
        }
    }
}

private struct StocksAssetRow: View {
    let asset: MarketAsset
    var isLast = false

    var body: some View {
        MarketStockRow(
            name: AssetDisplayName.format(catalogName: asset.name, kind: asset.kind),
            subtitle: asset.ticker,
            mark: StockMark(
                symbol: asset.symbol, displayName: asset.name, assetKind: asset.kind, logoURL: asset.logoURL),
            isAvailable: asset.isTradable,
            isLast: isLast
        ) {
            if asset.isPaused { PausedTag() }
            HStack(spacing: MonacoTheme.Space.xs) {
                if !asset.session.isRegularSession {
                    Image(systemName: "moon.fill").font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.warning).accessibilityLabel("Market closed")
                }
                if let micros = asset.priceMicros {
                    MoneyText(micros: micros, style: .row)
                } else {
                    Text("—").font(MonacoTheme.Typo.headline).foregroundStyle(MonacoTheme.muted)
                }
            }
            if let basisPoints = asset.changeBasisPoints {
                PercentText(basisPoints: basisPoints, style: .caption)
            } else {
                Text("—").font(MonacoTheme.Typo.subhead).foregroundStyle(MonacoTheme.muted)
            }
        }
    }
}
