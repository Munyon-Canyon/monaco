import MonacoCore
import SwiftUI

@MainActor
struct ProposeBuyStockView: View {
    let groupId: String
    let pot: ProposePot
    let service: ProposeService
    var onProposed: ((_ proposalId: String) -> Void)?

    @Environment(AppEnvironment.self) private var environment
    @State private var model: MonacoCore.StocksTabModel?
    @State private var query = ""
    @State private var picked: ProposeStock?

    private var trimmedQuery: String { query.trimmingCharacters(in: .whitespacesAndNewlines) }

    var body: some View {
        Group {
            if let model {
                content(model)
            }
        }
        .navigationTitle("Buy")
        .navigationBarTitleDisplayMode(.inline)
        .navigationDestination(item: $picked) { stock in
            ProposeAmountScreen(
                service: MonacoCore.LiveProposeService(api: environment.api), cabalID: groupId, stock: stock,
                potMicros: pot.totalMicros
            )
        }
        .task {
            let model = preparedModel()
            if model.rows.isEmpty { await model.load() }
        }
        .onChange(of: query) { _, query in model?.setQuery(query) }
        .accessibilityIdentifier("propose-buy-stock")
    }

    private func preparedModel() -> MonacoCore.StocksTabModel {
        if let model { return model }
        let created = MonacoCore.StocksTabModel(
            api: environment.api, hints: environment.hints, clock: ContinuousClock())
        model = created
        return created
    }

    @ViewBuilder
    private func content(_ model: MonacoCore.StocksTabModel) -> some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.l) {
            MonacoSearchField(placeholder: "Search Apple, Tesla, NVDA…", text: $query)
                .padding(.horizontal, MonacoTheme.Space.m)
            switch model.phase {
            case .idle where model.rows.isEmpty:
                ProposeStockSkeleton(rows: 5)
            case .loading where model.rows.isEmpty:
                ProposeStockSkeleton(rows: 5)
            case .failed where model.rows.isEmpty:
                EmptyState(title: "Couldn't load stocks.", actionTitle: "Try again") {
                    Task { await model.load() }
                }
            default:
                if model.rows.isEmpty, !trimmedQuery.isEmpty {
                    EmptyState(title: "No stocks match “\(trimmedQuery)”")
                } else {
                    if trimmedQuery.isEmpty {
                        MonacoSectionHeader("Popular").padding(.horizontal, MonacoTheme.Space.m)
                    }
                    MonacoGroupedList {
                        ForEach(Array(model.rows.enumerated()), id: \.element.id) { index, asset in
                            Button {
                                ProposeBuyStockSelection.select(asset, pick: { picked = $0 })
                            } label: {
                                ProposeBuyStockRow(asset: asset, isLast: index == model.rows.count - 1)
                            }
                            .buttonStyle(.monacoRow)
                            .disabled(!asset.isTradable)
                            .accessibilityIdentifier("propose-buy-stock-\(asset.symbol)")
                        }
                    }
                }
            }
        }
        .padding(.vertical, MonacoTheme.Space.s)
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
        .monacoCanvas()
    }

    private func finish(_ proposalId: String) {
        onProposed?(proposalId)
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

private struct ProposeBuyStockRow: View {
    let asset: MarketAsset
    let isLast: Bool

    var body: some View {
        HStack(spacing: MonacoTheme.Space.sm) {
            StockMark(
                symbol: asset.symbol, displayName: asset.name, assetKind: asset.kind, size: 46, logoURL: asset.logoURL
            )
            .frame(width: 46, height: 46)
            VStack(alignment: .leading, spacing: 2) {
                Text(asset.ticker).font(MonacoTheme.Typo.ticker)
                Text(asset.isTradable ? asset.name : "\(asset.name) · Can't buy right now")
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            if let price = asset.priceMicros {
                VStack(alignment: .trailing, spacing: 3) {
                    MoneyText(micros: price, style: .row, voice: .market)
                    Text(asset.changeText).font(MonacoTheme.Typo.caption).foregroundStyle(MonacoTheme.muted)
                }
            }
        }
        .padding(.horizontal, MonacoTheme.Space.m)
        .padding(.vertical, MonacoTheme.Space.s)
        .frame(minHeight: 64)
        .opacity(asset.isTradable ? 1 : 0.45)
        .overlay(alignment: .bottom) { if !isLast { MonacoRule().padding(.leading, 70) } }
        .accessibilityElement(children: .combine)
    }
}
