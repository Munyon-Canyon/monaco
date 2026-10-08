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
            Color.clear
            if let model { StocksTabScreen(model: model, open: open) }
        }
        .navigationTitle(StocksTab.title)
        .navigationBarTitleDisplayMode(.large)
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
                .padding(.horizontal, MonacoTheme.Space.m)
            MonacoChipBar(
                items: MonacoCore.StocksTabModel.Browse.allCases, selected: model.browse, title: \.title,
                identifierPrefix: "stocks-chip"
            ) { browse in
                Task { await model.show(browse) }
            }
            ScrollView { content }
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
                    EmptyState(title: "No stocks match “\(query)”")
                        .accessibilityIdentifier("assets-search-empty")
                } else {
                    EmptyState(title: "No stocks to show")
                }
            }
        }
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
            .overlay(alignment: .top) { MonacoRule() }
            .overlay(alignment: .bottom) { MonacoRule() }
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
        .padding(.horizontal, MonacoTheme.Space.m).padding(.vertical, MonacoTheme.Space.s).frame(
            minHeight: MonacoRowLayout.minHeight
        )
        .overlay(alignment: .bottom) {
            if !isLast {
                MonacoRule().padding(
                    .leading, MonacoTheme.Space.m + StockListRow.markSize + MonacoTheme.Space.sm)
            }
        }
        .contentShape(Rectangle())
        .accessibilityElement(children: .combine)
    }

    private var wide: some View {
        HStack(spacing: MonacoTheme.Space.sm) {
            mark
            names
            Spacer(minLength: MonacoTheme.Space.s)
            figures
        }
    }

    private var stacked: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            HStack(spacing: MonacoTheme.Space.sm) {
                mark
                names
            }
            figures.frame(maxWidth: .infinity, alignment: .trailing)
        }
    }

    private var mark: some View {
        StockMark(
            symbol: asset.symbol, displayName: asset.name, assetKind: asset.kind, size: StockListRow.markSize,
            logoURL: asset.logoURL
        )
        .frame(width: StockListRow.markSize, height: StockListRow.markSize)
    }

    private var names: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
            Text(asset.ticker).font(MonacoTheme.Typo.bodyStrong)
            Text(AssetDisplayName.format(catalogName: asset.name, kind: asset.kind))
                .font(MonacoTheme.Typo.caption).foregroundStyle(MonacoTheme.muted).lineLimit(1)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private var figures: some View {
        VStack(alignment: .trailing, spacing: MonacoTheme.Space.xs) {
            HStack(spacing: 4) {
                if !asset.session.isRegularSession {
                    Image(systemName: "moon.fill").font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.warning).accessibilityLabel("Market closed")
                }
                if let micros = asset.priceMicros {
                    MoneyText(micros: micros, style: .row, voice: .market)
                } else {
                    Text("—").font(MonacoTheme.Typo.bodyStrong).foregroundStyle(MonacoTheme.muted)
                }
            }
            if let basisPoints = asset.changeBasisPoints {
                PercentText(basisPoints: basisPoints, style: .caption)
            } else {
                Text("—").font(MonacoTheme.Typo.caption).foregroundStyle(MonacoTheme.muted)
            }
        }
    }
}

#if DEBUG
final class StocksTabSampleHarnessEntry: SampleHarnessEntry {
    @MainActor override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard arguments.contains("-MonacoStocksSectionsSample") else { return nil }
        return AnyView(StocksSectionsSampleView())
    }
}

private struct StocksSectionsSampleView: View {
    private let popular = MarketMapping.asset(.googl)
    private let preIpo = MarketMapping.asset(.spaceX)
    private let all = MarketMapping.asset(.unpriced)
    @State private var browse = MonacoCore.StocksTabModel.Browse.all

    var body: some View {
        NavigationStack {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                MonacoChipBar(
                    items: MonacoCore.StocksTabModel.Browse.allCases, selected: browse, title: \.title,
                    identifierPrefix: "stocks-chip"
                ) { browse = $0 }
                ScrollView { sampleRows(rowsForBrowse) }
            }
            .navigationTitle(StocksTab.title)
            .navigationBarTitleDisplayMode(.large)
            .monacoCanvas()
        }
    }

    private var rowsForBrowse: [MarketAsset] {
        switch browse {
        case .all: [popular, preIpo, all]
        case .popular: [popular]
        case .preIpo: [preIpo]
        }
    }

    private func sampleRows(_ assets: [MarketAsset]) -> some View {
        MonacoGroupedList {
            ForEach(assets) { StocksAssetRow(asset: $0, isLast: $0.id == assets.last?.id) }
        }
    }
}
#endif
