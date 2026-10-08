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
    @State private var model: ProposeFromAssetModel?

    var body: some View {
        Group {
            switch model?.state ?? .loading {
            case .idle, .loading: MonacoRowSkeleton(rows: 3, markShape: .tile, hasTrailing: false)
            case .failed:
                MonacoErrorRow(thing: "cabals", identifier: "propose-from-asset-error") { Task { await model?.load() } }
            case .loaded(let cabals):
                if cabals.isEmpty {
                    EmptyState(title: "Join a cabal first", actionTitle: "Browse cabals") {
                        environment.navigator.selectedTab = .cabals
                    }
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
            await created.load()
        }
    }

    private func pick(_ cabals: [Components.Schemas.MyCabal]) -> some View {
        ScrollView {
            MonacoGroupedList {
                ForEach(cabals, id: \.id) { cabal in
                    NavigationLink {
                        amount(cabal)
                    } label: {
                        ProposePickCabalRow(name: cabal.name, cabalID: cabal.id, isLast: cabal.id == cabals.last?.id)
                    }
                    .buttonStyle(.monacoRow)
                    .accessibilityIdentifier("propose-pick-cabal-\(cabal.id)")
                }
            }
            .padding(.top, MonacoTheme.Space.s)
        }
        .monacoCanvas()
        .navigationTitle(pickTitle)
        .navigationBarTitleDisplayMode(.inline)
    }

    private var pickTitle: String {
        "Which cabal should \(kind == .buy ? "buy" : "sell") \(AssetSymbolFormatter.display(symbol))?"
    }

    @ViewBuilder
    private func amount(_ cabal: Components.Schemas.MyCabal) -> some View {
        switch kind {
        case .buy:
            ProposeAmountScreen(
                service: MonacoCore.LiveProposeService(api: environment.api), cabalID: cabal.id,
                stock: ProposeStock(symbol: symbol))
        case .sell:
            CabalPotModelHost(cabalID: cabal.id) { pot in
                ProposeSellView(pot: pot, initialSymbol: symbol)
            }
        }
    }
}

nonisolated enum ProposeKind: Hashable, Sendable {
    case buy, sell
}

private struct ProposePickCabalRow: View {
    let name: String
    let cabalID: String
    let isLast: Bool

    var body: some View {
        MonacoRow(title: name, chevron: true, isLast: isLast) {
            EmptyView()
        } trailing: {
            CabalPotModelHost(cabalID: cabalID) { pot in
                if let summary = pot?.summary {
                    Text(summary.potValue).font(MonacoTheme.Typo.caption).foregroundStyle(MonacoTheme.muted)
                } else {
                    SkeletonBlock(width: 56, height: 12)
                }
            }
        }
    }
}
