import MonacoAPI
import MonacoCore
import SwiftUI

struct StocksTabView: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @State private var model: MonacoCore.StocksTabModel?

    init(model: MonacoCore.StocksTabModel? = nil) { _model = State(initialValue: model) }

    var body: some View {
        Group { if let model { StocksTabScreen(model: model, open: open) } }
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
        if model.popular.rows.isEmpty && model.preIpo.rows.isEmpty && model.all.rows.isEmpty { await model.load() }
        await model.observe()
    }

    private func preparedModel() -> MonacoCore.StocksTabModel {
        if let model { return model }
        let created = MonacoCore.StocksTabModel(
            api: environment.api, hints: environment.hints, clock: ContinuousClock())
        model = created
        return created
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
        if model.isSearching {
            search
        } else if hasRows || hasPendingSections {
            section("Popular", state: model.popular)
            section("Pre-IPO", state: model.preIpo)
            section("All stocks", state: model.all, loadsMore: true)
        } else if hasFailure {
            EmptyState(title: "Couldn't load stocks.", actionTitle: "Try again") { Task { await model.load() } }
                .accessibilityIdentifier("assets-failed")
        } else {
            EmptyState(title: "No stocks to show")
        }
    }

    private var sections: [MonacoCore.StocksTabModel.SectionState] { [model.popular, model.preIpo, model.all] }
    private var hasRows: Bool { sections.contains { !$0.rows.isEmpty } }
    private var hasPendingSections: Bool { sections.contains { $0.phase == .idle || $0.phase == .loading } }
    private var hasFailure: Bool { sections.contains { if case .failed = $0.phase { true } else { false } } }

    @ViewBuilder private var search: some View {
        if !model.search.rows.isEmpty {
            rows(model.search.rows, loadsMore: model.displayedHasMore)
        } else {
            switch model.search.phase {
            case .loading:
                BoardRowSkeleton()
            case .failed:
                EmptyState(title: "Couldn't load stocks.", actionTitle: "Try again") { Task { await model.load() } }
            default:
                Text("No stocks match “\(query)”")
                    .font(MonacoTheme.Typo.body)
                    .foregroundStyle(MonacoTheme.muted)
                    .frame(maxWidth: .infinity)
                    .padding(.vertical, MonacoTheme.Space.xl)
                    .accessibilityIdentifier("assets-search-empty")
            }
        }
    }

    @ViewBuilder private func section(
        _ title: String, state: MonacoCore.StocksTabModel.SectionState, loadsMore: Bool = false
    )
        -> some View
    {
        if !state.rows.isEmpty {
            MonacoSectionHeader(title).padding(.horizontal, MonacoTheme.Space.m)
            rows(state.rows, loadsMore: loadsMore)
        } else if state.phase == .loading {
            MonacoSectionHeader(title).padding(.horizontal, MonacoTheme.Space.m)
            BoardRowSkeleton()
        } else {
            EmptyView()
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
                        Task {
                            if model.isSearching { await model.loadMoreDisplayed() } else { await model.loadMore() }
                        }
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
        .padding(.horizontal, MonacoTheme.Space.m).padding(.vertical, MonacoTheme.Space.s).frame(minHeight: 64)
        .overlay(alignment: .bottom) {
            if !isLast {
                Rectangle().fill(MonacoTheme.hairline).frame(height: 1).padding(
                    .leading, 46 + MonacoTheme.Space.sm + MonacoTheme.Space.m)
            }
        }
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
            symbol: asset.symbol, displayName: asset.name, assetKind: asset.kind, size: 46, logoURL: asset.logoURL
        )
        .frame(width: 46, height: 46)
    }

    private var names: some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(asset.ticker).font(MonacoTheme.Typo.bodyStrong)
            Text(AssetDisplayName.format(catalogName: asset.name, kind: asset.kind))
                .font(MonacoTheme.Typo.caption).foregroundStyle(MonacoTheme.muted).lineLimit(1)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private var figures: some View {
        VStack(alignment: .trailing, spacing: 3) {
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

    var body: some View {
        NavigationStack {
            ScrollView {
                MonacoSectionHeader("Popular").padding(.horizontal, MonacoTheme.Space.m)
                sampleRows([popular])
                MonacoSectionHeader("Pre-IPO").padding(.horizontal, MonacoTheme.Space.m)
                sampleRows([preIpo])
                MonacoSectionHeader("All stocks").padding(.horizontal, MonacoTheme.Space.m)
                sampleRows([popular, all])
            }
            .navigationTitle(StocksTab.title)
            .navigationBarTitleDisplayMode(.large)
            .monacoCanvas()
        }
    }

    private func sampleRows(_ assets: [MarketAsset]) -> some View {
        MonacoGroupedList {
            ForEach(assets) { StocksAssetRow(asset: $0, isLast: $0.id == assets.last?.id) }
        }
    }
}
#endif
