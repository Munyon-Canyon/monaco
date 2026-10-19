import MonacoCore
import SwiftUI

@MainActor
struct ProposeBuyStockView: View {
    let cabalID: String

    @Environment(AppEnvironment.self) private var environment
    @State private var model: MonacoCore.StocksTabModel?
    @State private var context = MonacoCore.ProposeCabalContext()
    @State private var query = ""
    @State private var sort = ProposeSort.volume
    @State private var filter = MonacoCore.StocksTabModel.Browse.all
    @State private var picked: ProposeStock?
    private let makeModel: @MainActor (AppEnvironment) -> MonacoCore.StocksTabModel

    init(
        cabalID: String, initialQuery: String = "",
        makeModel: @escaping @MainActor (AppEnvironment) -> MonacoCore.StocksTabModel = ProposeBuyStockView.liveModel
    ) {
        self.cabalID = cabalID
        _query = State(initialValue: initialQuery)
        self.makeModel = makeModel
    }

    private var trimmedQuery: String { query.trimmingCharacters(in: .whitespacesAndNewlines) }

    var body: some View {
        VStack(spacing: 0) {
            if let model {
                content(model)
            }
        }
        .navigationTitle(ProposeFlowCopy.buyListTitle)
        .navigationBarTitleDisplayMode(.inline)
        .navigationDestination(item: $picked) { stock in
            ProposeAmountScreen(
                service: MonacoCore.LiveProposeService(api: environment.api), cabalID: cabalID, stock: stock,
                bestChains: environment.bestChains)
        }
        .task {
            let model = preparedModel()
            if !trimmedQuery.isEmpty { model.setQuery(query) }
            if model.rows.isEmpty { await model.load() }
        }
        .task { await context.load(api: environment.api, cabalID: cabalID) }
        .onChange(of: query) { _, query in model?.setQuery(query) }
        .onChange(of: filter) { _, filter in
            Task { await model?.show(filter) }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("propose-buy-stock")
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

    @ViewBuilder
    private func content(_ model: MonacoCore.StocksTabModel) -> some View {
        let isLoading = model.rows.isEmpty && trimmedQuery.isEmpty && !isFailed(model) && model.phase != .loaded
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            if isLoading {
                ScrollView { ProposeStockSkeleton(rows: 4) }
            } else if isFailed(model), model.rows.isEmpty {
                EmptyState.loadFailed(identifier: "propose-buy-stock-error") { Task { await model.load() } }
                    .frame(maxHeight: .infinity, alignment: .top)
            } else {
                MonacoSearchField(placeholder: ProposeFlowCopy.buySearchPlaceholder, text: $query)
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                ProposeStockControls(sort: $sort, filter: $filter)
                results(model)
            }
        }
        .padding(.top, MonacoTheme.Space.sm)
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
        .monacoCanvas()
    }

    private func isFailed(_ model: MonacoCore.StocksTabModel) -> Bool {
        if case .failed = model.phase { return true }
        return false
    }

    @ViewBuilder
    private func results(_ model: MonacoCore.StocksTabModel) -> some View {
        let assets = sort.apply(model.rows).pausedLast
        if assets.isEmpty {
            EmptyState(
                title: ProposeFlowCopy.noStocksTitle, message: ProposeFlowCopy.noStocksMessage,
                systemImage: MonacoIcon.search.symbol
            )
            .frame(maxHeight: .infinity, alignment: .top)
        } else {
            ScrollView {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                    Text(context.name ?? " ")
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.muted)
                        .padding(.horizontal, MonacoTheme.Space.gutter)
                    stocks(assets)
                }
            }
        }
    }

    private func stocks(_ assets: [MarketAsset]) -> some View {
        MonacoGroupedList {
            ForEach(Array(assets.enumerated()), id: \.element.id) { index, asset in
                Button {
                    ProposeBuyStockSelection.select(asset, pick: { picked = $0 })
                } label: {
                    ProposeStockRow(
                        stock: ProposeStock(listing: asset), logoURL: asset.logoURL,
                        isLast: index == assets.count - 1)
                }
                .buttonStyle(.monacoRow)
                .disabled(!asset.isTradable)
                .accessibilityActivationPoint(.center)
                .accessibilityIdentifier("propose-buy-stock-\(asset.symbol)")
            }
        }
    }
}

enum ProposeBuyStockSelection {
    static func select(_ asset: MarketAsset, pick: (ProposeStock) -> Void) {
        guard asset.isTradable else { return }
        pick(
            ProposeStock(
                symbol: asset.symbol, name: asset.name, priceMicros: asset.priceMicros,
                isTradable: asset.isTradable, isQuotable: asset.isQuotable, assetKind: asset.kind
            ))
    }
}

extension ProposeStock {
    fileprivate init(listing asset: MarketAsset) {
        self.init(
            symbol: asset.symbol, name: asset.name, priceMicros: asset.priceMicros,
            change24h: asset.changeBasisPoints.map(Self.ratio(basisPoints:)),
            isTradable: asset.isTradable, isQuotable: asset.isQuotable, assetKind: asset.kind)
    }

    private static func ratio(basisPoints: Int64) -> String {
        let magnitude = basisPoints.magnitude
        let fraction = String(magnitude % 10_000)
        let padded = String(repeating: "0", count: 4 - fraction.count) + fraction
        return "\(basisPoints < 0 ? "-" : "")\(magnitude / 10_000).\(padded)"
    }
}
