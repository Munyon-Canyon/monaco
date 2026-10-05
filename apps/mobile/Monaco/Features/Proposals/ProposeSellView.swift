import MonacoAPI
import MonacoCore
import SwiftUI

struct ProposeSellView: View {
    let pot: CabalPotModel?
    var initialSymbol: String?

    @Environment(AppEnvironment.self) private var environment
    @State private var picked: ProposeHolding?
    @State private var didOpenInitialSymbol = false

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                MonacoSectionHeader("What the cabal owns").padding(.horizontal, MonacoTheme.Space.m)
                content
            }
            .padding(.vertical, MonacoTheme.Space.s)
        }
        .monacoCanvas()
        .navigationTitle("Sell")
        .navigationBarTitleDisplayMode(.inline)
        .navigationDestination(item: $picked) { holding in
            if let pot {
                ProposeAmountScreen(
                    service: LiveProposeService(api: environment.api), cabalID: pot.cabalID,
                    stock: ProposeStock(holding: holding), trade: .sell(holding))
            }
        }
        .onChange(of: pot?.summary?.sellable, initial: true) { _, holdings in
            guard !didOpenInitialSymbol, let initialSymbol, let holdings else { return }
            didOpenInitialSymbol = true
            picked = holdings.first { $0.symbol.caseInsensitiveCompare(initialSymbol) == .orderedSame }
        }
        .accessibilityIdentifier("propose-sell")
    }

    @ViewBuilder private var content: some View {
        switch pot?.state ?? .loading {
        case .idle, .loading:
            ProposeStockSkeleton(rows: 3)
        case .failed:
            EmptyState(title: "Couldn't load holdings.", actionTitle: "Try again") { Task { await pot?.load() } }
        case .loaded(let summary):
            if summary.sellable.isEmpty {
                EmptyState(title: "Nothing to sell yet")
            } else {
                MonacoGroupedList {
                    ForEach(summary.sellable) { holding in
                        Button {
                            picked = holding
                        } label: {
                            ProposeHoldingRow(holding: holding, isLast: holding.id == summary.sellable.last?.id)
                        }
                        .buttonStyle(.monacoRow)
                        .accessibilityIdentifier("propose-sell-\(holding.symbol)")
                    }
                }
            }
        }
    }
}

private struct ProposeHoldingRow: View {
    let holding: ProposeHolding
    let isLast: Bool

    var body: some View {
        MonacoRow(
            title: holding.ticker, titleFont: MonacoTheme.Typo.ticker, subtitle: holding.detail, chevron: true,
            isLast: isLast
        ) {
            StockMark(symbol: holding.symbol, displayName: holding.name, assetKind: holding.kind, logoURL: nil)
        } trailing: {
            MoneyText(micros: holding.valueMicros, style: .row)
        }
    }
}

extension ProposeStock {
    init(holding: ProposeHolding) {
        self.init(
            symbol: holding.symbol, name: holding.name, priceMicros: holding.priceMicros, assetKind: holding.kind)
    }
}
