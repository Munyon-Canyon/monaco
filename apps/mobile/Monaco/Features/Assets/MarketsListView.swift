import MonacoCore
import SwiftUI

struct MarketsListView: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(\.hostMainTab) private var hostMainTab
    @State private var model: MonacoCore.MarketsListModel?
    private let order: MonacoCore.MarketsListModel.Order

    init(order: MonacoCore.MarketsListModel.Order) {
        self.order = order
    }

    var body: some View {
        ZStack {
            if let model {
                content(model)
            } else {
                ScrollView { MarketsLoading() }
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
        .monacoCanvas()
        .foregroundStyle(MonacoTheme.primaryText)
        .navigationTitle("Stocks")
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            if let model {
                ToolbarItem(placement: .topBarTrailing) { sortMenu(model) }
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("markets-list-root")
        .task { await start() }
        .onChange(of: model?.failureTick) { _, _ in
            guard let error = model?.lastError else { return }
            toasts.show(error)
        }
    }

    @ViewBuilder private func content(_ model: MonacoCore.MarketsListModel) -> some View {
        if case .failed = model.phase {
            AssetsLoadFailure(identifier: "markets-list-failed") { Task { await model.load() } }
        } else if model.rows.isEmpty, model.phase != .loaded {
            ScrollView { MarketsLoading() }
        } else {
            ScrollView {
                LazyVStack(spacing: 0) {
                    ForEach(model.rows) { asset in
                        MarketsWatchRow(
                            asset: asset, subline: asset.ticker, isLast: asset.id == model.rows.last?.id,
                            identifier: "markets-list-row-\(asset.symbol)", open: open
                        )
                        .onAppear {
                            guard asset.id == model.rows.last?.id else { return }
                            Task { await model.loadMore() }
                        }
                    }
                }
            }
            .accessibilityIdentifier("markets-list")
        }
    }

    private func sortMenu(_ model: MonacoCore.MarketsListModel) -> some View {
        Menu {
            Picker(
                "Sort",
                selection: Binding(get: { model.order }, set: { next in Task { await model.select(next) } })
            ) {
                ForEach(MonacoCore.MarketsListModel.Order.allCases, id: \.self) { order in
                    Text(order.title).tag(order)
                }
            }
        } label: {
            Image(systemName: "arrow.up.arrow.down")
        }
        .accessibilityLabel("Sort")
        .accessibilityIdentifier("markets-list-sort")
    }

    private func start() async {
        let created = model ?? MonacoCore.MarketsListModel(api: environment.api, order: order)
        model = created
        guard created.rows.isEmpty else { return }
        await created.load()
    }

    private func open(_ asset: MarketAsset) {
        Haptics.selection()
        environment.navigator.open(AssetRoute(symbol: asset.symbol, seed: AssetSeed(asset)), in: hostMainTab ?? .stocks)
    }
}
