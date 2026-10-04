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
            case .idle, .loading: ProgressView()
            case .failed:
                EmptyState(title: "Couldn't load cabals.", actionTitle: "Try again") { Task { await model?.load() } }
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
        List(cabals, id: \.id) { cabal in
            NavigationLink(cabal.name) { amount(cabal) }
        }
        .navigationTitle("Which cabal should \(kind == .buy ? "buy" : "sell") \(symbol)?")
    }

    private func amount(_ cabal: Components.Schemas.MyCabal) -> some View {
        ProposeAmountScreen(
            service: MonacoCore.LiveProposeService(api: environment.api), cabalID: cabal.id,
            stock: ProposeStock(symbol: symbol), isSell: kind == .sell)
    }
}

nonisolated enum ProposeKind: Hashable, Sendable {
    case buy, sell
}
