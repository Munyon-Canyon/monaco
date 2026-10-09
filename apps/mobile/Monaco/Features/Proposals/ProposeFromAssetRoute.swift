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
            case .idle, .loading: framed { MonacoRowSkeleton(rows: 3, markShape: .tile, hasTrailing: false) }
            case .failed:
                framed {
                    MonacoErrorRow(thing: "cabals", identifier: "propose-from-asset-error") {
                        Task { await model?.load() }
                    }
                }
            case .loaded(let cabals):
                if cabals.isEmpty {
                    framed {
                        EmptyState(title: "Join a cabal first", actionTitle: "Find a cabal") {
                            environment.navigator.selectedTab = .cabals
                        }
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

    private func framed<Content: View>(@ViewBuilder _ content: () -> Content) -> some View {
        content()
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
            .monacoCanvas()
            .navigationTitle("Pick a cabal")
            .navigationBarTitleDisplayMode(.inline)
    }

    private func pick(_ cabals: [Components.Schemas.MyCabal]) -> some View {
        framed {
            ScrollView {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                    Text(pickTitle)
                        .font(MonacoTheme.Typo.title)
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
                .padding(.top, MonacoTheme.Space.s)
            }
        }
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
    let cabal: Components.Schemas.MyCabal
    let isLast: Bool

    var body: some View {
        MonacoRow(title: cabal.name, chevron: true, isLast: isLast) {
            CabalMark(groupId: cabal.id, name: cabal.name, pictureUrl: cabal.pictureUrl)
        } trailing: {
            CabalPotModelHost(cabalID: cabal.id) { pot in
                if let summary = pot?.summary {
                    Text(summary.potValue).font(MonacoTheme.Typo.caption).foregroundStyle(MonacoTheme.muted)
                } else {
                    SkeletonBlock(width: 56, height: 12)
                }
            }
        }
    }
}
