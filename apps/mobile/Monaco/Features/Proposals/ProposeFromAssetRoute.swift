import MonacoAPI
import MonacoCore
import SwiftUI

nonisolated struct ProposeFromAssetRoute: AppRoute {
    let symbol: String
    let kind: ProposeKind

    @MainActor func destination() -> some View {
        ProposeFromAssetScreen(symbol: symbol, kind: kind)
    }
}

struct ProposeFromAssetScreen: View {
    let symbol: String
    let kind: ProposeKind
    @Environment(AppEnvironment.self) private var environment
    @Environment(\.hostMainTab) private var hostMainTab
    @State private var model: ProposeFromAssetModel?

    var body: some View {
        Group {
            switch model?.state ?? .loading {
            case .idle, .loading: framed { MonacoRowSkeleton(rows: 4, markShape: .tile, hasTrailing: false) }
            case .failed:
                framed {
                    EmptyState.loadFailed(identifier: "propose-from-asset-error") {
                        Task { await model?.load(kind: kind, symbol: symbol) }
                    }
                }
            case .loaded(let cabals):
                if cabals.isEmpty {
                    framed { emptyState }
                } else if cabals.count == 1 {
                    amount(cabals[0])
                } else {
                    pick(cabals)
                }
            }
        }
        .task {
            let created = model ?? ProposeFromAssetModel(api: environment.api)
            model = created
            guard case .idle = created.state else { return }
            await created.load(kind: kind, symbol: symbol)
        }
    }

    @ViewBuilder private var emptyState: some View {
        if model?.destination == .votersOnly {
            EmptyState(title: "Only voters can propose")
        } else {
            EmptyState(
                title: "Join a cabal first", actionTitle: "Find a cabal", systemImage: MonacoIcon.cabals.symbol
            ) {
                let tab = hostMainTab ?? environment.navigator.selectedTab
                environment.navigator.closeProposeFlow(in: tab)
                environment.navigator.selectedTab = .cabals
            }
        }
    }

    private func framed<Content: View>(@ViewBuilder _ content: () -> Content) -> some View {
        content()
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
            .monacoCanvas()
            .navigationTitle(ProposeFlowCopy.chooseCabalTitle)
            .navigationBarTitleDisplayMode(.inline)
    }

    private func pick(_ cabals: [Components.Schemas.MyCabal]) -> some View {
        framed {
            ScrollView {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                    Text(pickTitle)
                        .font(MonacoTheme.Typo.section)
                        .foregroundStyle(MonacoTheme.ink)
                        .fixedSize(horizontal: false, vertical: true)
                        .padding(.horizontal, MonacoTheme.Space.gutter)
                        .accessibilityIdentifier("propose-pick-cabal-question")
                    MonacoGroupedList {
                        ForEach(cabals, id: \.id) { cabal in
                            NavigationLink {
                                amount(cabal)
                            } label: {
                                ProposePickCabalRow(cabal: cabal, isLast: cabal.id == cabals.last?.id)
                            }
                            .buttonStyle(.monacoRow)
                            .accessibilityIdentifier("propose-pick-cabal-\(cabal.id)")
                        }
                    }
                }
                .padding(.top, MonacoTheme.Space.m)
            }
        }
    }

    private var pickTitle: String {
        ProposeFlowCopy.chooseCabalQuestion(
            verb: kind == .buy ? "buy" : "sell", ticker: AssetSymbolFormatter.display(symbol))
    }

    @ViewBuilder
    private func amount(_ cabal: Components.Schemas.MyCabal) -> some View {
        switch kind {
        case .buy:
            ProposeAmountScreen(
                service: MonacoCore.LiveProposeService(api: environment.api), cabalID: cabal.id,
                stock: ProposeStock(symbol: symbol), bestChains: environment.bestChains)
        case .sell:
            CabalPotModelHost(cabalID: cabal.id) { pot in
                sellAmount(cabal, pot: pot)
            }
        }
    }

    @ViewBuilder
    private func sellAmount(_ cabal: Components.Schemas.MyCabal, pot: CabalPotModel?) -> some View {
        if let holding = pot?.summary?.sellable.first(where: {
            $0.symbol.caseInsensitiveCompare(symbol) == .orderedSame
        }) {
            ProposeAmountScreen(
                service: MonacoCore.LiveProposeService(api: environment.api), cabalID: cabal.id,
                stock: ProposeStock(holding: holding), trade: .sell(holding))
        } else if case .failed = pot?.state {
            framed {
                EmptyState.loadFailed(identifier: "propose-sell-error") { Task { await pot?.load() } }
            }
        } else if case .loaded = pot?.state {
            framed { EmptyState(title: ProposeFlowCopy.noHoldingsToSell, systemImage: MonacoIcon.wallet.symbol) }
        } else {
            framed { MonacoRowSkeleton(rows: 4, markShape: .tile, hasTrailing: false) }
        }
    }
}

private struct ProposePickCabalRow: View {
    let cabal: Components.Schemas.MyCabal
    let isLast: Bool

    var body: some View {
        CabalPotModelHost(cabalID: cabal.id) { pot in
            MonacoRow(
                title: cabal.name, subtitle: pot?.summary.map { "\($0.cash) cash" }, chevron: true,
                isLast: isLast
            ) {
                CabalMark(
                    groupId: cabal.id, name: cabal.name, size: MonacoRowLayout.baseMarkSize,
                    pictureUrl: cabal.pictureUrl)
            }
        }
    }
}
