import MonacoAPI
import MonacoCore
import SwiftUI

struct ProposeSellView: View {
    let pot: CabalPotModel?
    var initialSymbol: String?

    @Environment(AppEnvironment.self) private var environment
    @Environment(\.hostMainTab) private var hostMainTab
    @State private var context = MonacoCore.ProposeCabalContext()
    @State private var picked: ProposeHolding?
    @State private var logos: [String: URL] = [:]
    @State private var didOpenInitialSymbol = false
    @State private var query = ""
    @State private var sort = ProposeSort.volume
    @State private var filter = MonacoCore.StocksTabModel.Browse.all

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            content
        }
        .padding(.top, MonacoTheme.Space.sm)
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
        .monacoCanvas()
        .navigationTitle(ProposeFlowCopy.sellListTitle)
        .navigationBarTitleDisplayMode(.inline)
        .navigationDestination(item: $picked) { holding in
            if let pot {
                ProposeAmountScreen(
                    service: LiveProposeService(api: environment.api), cabalID: pot.cabalID,
                    stock: ProposeStock(holding: holding, logoURL: logos[holding.symbol]), trade: .sell(holding))
            }
        }
        .onChange(of: pot?.summary?.sellable, initial: true) { _, holdings in
            guard !didOpenInitialSymbol, let initialSymbol, let holdings else { return }
            didOpenInitialSymbol = true
            picked = holdings.first { $0.symbol.caseInsensitiveCompare(initialSymbol) == .orderedSame }
        }
        .task { await context.load(api: environment.api, cabalID: pot?.cabalID ?? "") }
        .task(id: pot?.summary?.sellable) {
            guard let holdings = pot?.summary?.sellable else { return }
            logos = await environment.assetLogos.logos(for: holdings.map(\.symbol))
        }
        .accessibilityIdentifier("propose-sell")
    }

    @ViewBuilder private var content: some View {
        switch pot?.state ?? .loading {
        case .idle, .loading:
            ScrollView { ProposeStockSkeleton(rows: 3) }
        case .failed:
            EmptyState.loadFailed(identifier: "propose-sell-error") { Task { await pot?.load() } }
                .frame(maxHeight: .infinity, alignment: .top)
        case .loaded(let summary):
            if summary.sellable.isEmpty {
                EmptyState(
                    title: ProposeFlowCopy.noHoldingsToSell, message: ProposeFlowCopy.noPositionsMessage,
                    actionTitle: ProposeFlowCopy.backToCabal, systemImage: MonacoIcon.wallet.symbol
                ) {
                    environment.navigator.closeProposeFlow(in: hostMainTab ?? environment.navigator.selectedTab)
                }
                .frame(maxHeight: .infinity, alignment: .top)
            } else {
                list(summary.sellable)
            }
        }
    }

    private func list(_ holdings: [ProposeHolding]) -> some View {
        let shown = visible(holdings)
        return VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            MonacoSearchField(placeholder: ProposeFlowCopy.sellSearchPlaceholder, text: $query)
                .padding(.horizontal, MonacoTheme.Space.gutter)
            ProposeStockControls(
                sort: $sort, filter: $filter, sorts: [.volume, .name], filters: [.all, .preIpo])
            if shown.isEmpty {
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
                        MonacoGroupedList {
                            ForEach(shown) { holding in
                                Button {
                                    picked = holding
                                } label: {
                                    ProposeHoldingRow(
                                        holding: holding, logoURL: logos[holding.symbol],
                                        isLast: holding.id == shown.last?.id)
                                }
                                .buttonStyle(.monacoRow)
                                .accessibilityIdentifier("propose-sell-\(holding.symbol)")
                            }
                        }
                    }
                }
            }
        }
    }

    private func visible(_ holdings: [ProposeHolding]) -> [ProposeHolding] {
        let needle = query.trimmingCharacters(in: .whitespacesAndNewlines)
        let matching = holdings.filter { holding in
            (filter != .preIpo || holding.kind == .preIpo)
                && (needle.isEmpty || holding.name.localizedStandardContains(needle)
                    || holding.ticker.localizedStandardContains(needle))
        }
        return sort.apply(matching)
    }
}

private struct ProposeHoldingRow: View {
    let holding: ProposeHolding
    let logoURL: URL?
    let isLast: Bool

    var body: some View {
        MonacoRow(
            title: holding.name, subtitle: "\(holding.ticker) · \(holding.quantity(of: holding.tokenAmount))",
            chevron: true, isLast: isLast
        ) {
            StockMark(
                symbol: holding.symbol, displayName: holding.name, assetKind: holding.kind,
                size: MonacoRowLayout.baseMarkSize, logoURL: logoURL)
        }
    }
}

extension ProposeStock {
    init(holding: ProposeHolding, logoURL: URL? = nil) {
        self.init(
            symbol: holding.symbol, name: holding.name, priceMicros: holding.priceMicros, assetKind: holding.kind,
            logoURL: logoURL)
    }
}
