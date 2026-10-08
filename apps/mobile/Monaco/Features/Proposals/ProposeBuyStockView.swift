import MonacoCore
import SwiftUI

@MainActor
struct ProposeBuyStockView: View {
    let cabalID: String

    @Environment(AppEnvironment.self) private var environment
    @State private var model: MonacoCore.StocksTabModel?
    @State private var query = ""
    @State private var picked: ProposeStock?
    private let makeModel: @MainActor (AppEnvironment) -> MonacoCore.StocksTabModel

    init(
        cabalID: String,
        makeModel: @escaping @MainActor (AppEnvironment) -> MonacoCore.StocksTabModel = ProposeBuyStockView.liveModel
    ) {
        self.cabalID = cabalID
        self.makeModel = makeModel
    }

    private var trimmedQuery: String { query.trimmingCharacters(in: .whitespacesAndNewlines) }

    var body: some View {
        VStack(spacing: 0) {
            if let model {
                content(model)
            }
        }
        .navigationTitle("Buy")
        .navigationBarTitleDisplayMode(.inline)
        .navigationDestination(item: $picked) { stock in
            ProposeAmountScreen(
                service: MonacoCore.LiveProposeService(api: environment.api), cabalID: cabalID, stock: stock
            )
        }
        .task {
            let model = preparedModel()
            if model.rows.isEmpty { await model.load() }
        }
        .onChange(of: query) { _, query in model?.setQuery(query) }
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

    private func content(_ model: MonacoCore.StocksTabModel) -> some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.l) {
            MonacoSearchField(placeholder: "Search Apple, Tesla, NVDA…", text: $query)
                .padding(.horizontal, MonacoTheme.Space.m)
            ScrollView { results(model) }
        }
        .padding(.vertical, MonacoTheme.Space.s)
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
        .monacoCanvas()
    }

    @ViewBuilder
    private func results(_ model: MonacoCore.StocksTabModel) -> some View {
        switch model.phase {
        case .idle where model.rows.isEmpty:
            ProposeStockSkeleton(rows: 5)
        case .loading where model.rows.isEmpty:
            ProposeStockSkeleton(rows: 5)
        case .failed where model.rows.isEmpty:
            MonacoErrorRow(thing: "stocks", identifier: "propose-buy-stock-error") {
                Task { await model.load() }
            }
        default:
            if model.rows.isEmpty, !trimmedQuery.isEmpty {
                EmptyState(title: "No stocks match “\(trimmedQuery)”")
            } else {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.l) {
                    if trimmedQuery.isEmpty {
                        MonacoSectionHeader("Popular").padding(.horizontal, MonacoTheme.Space.m)
                    }
                    MonacoGroupedList {
                        ForEach(Array(model.rows.enumerated()), id: \.element.id) { index, asset in
                            Button {
                                ProposeBuyStockSelection.select(asset, pick: { picked = $0 })
                            } label: {
                                ProposeStockRow(
                                    stock: ProposeStock(listing: asset), logoURL: asset.logoURL,
                                    isLast: index == model.rows.count - 1)
                            }
                            .buttonStyle(.monacoRow)
                            .disabled(!asset.isTradable)
                            .accessibilityIdentifier("propose-buy-stock-\(asset.symbol)")
                        }
                    }
                }
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
                isTradable: asset.isTradable, assetKind: asset.kind
            ))
    }
}

extension ProposeStock {
    fileprivate init(listing asset: MarketAsset) {
        self.init(
            symbol: asset.symbol, name: asset.name, priceMicros: asset.priceMicros,
            change24h: asset.changeBasisPoints.map(Self.ratio(basisPoints:)),
            isTradable: asset.isTradable, assetKind: asset.kind)
    }

    private static func ratio(basisPoints: Int64) -> String {
        let magnitude = basisPoints.magnitude
        let fraction = String(magnitude % 10_000)
        let padded = String(repeating: "0", count: 4 - fraction.count) + fraction
        return "\(basisPoints < 0 ? "-" : "")\(magnitude / 10_000).\(padded)"
    }
}
