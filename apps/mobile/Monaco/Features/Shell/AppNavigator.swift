import SwiftUI

@Observable
@MainActor
final class AppNavigator {
    var selectedTab: MainTab = .home
    var homePath: [AnyAppRoute] = []
    var feedPath: [AnyAppRoute] = []
    var cabalsPath: [AnyAppRoute] = []
    var stocksPath: [AnyAppRoute] = []
    var profilePath: [AnyAppRoute] = []

    func reset() {
        selectedTab = .home
        homePath = []
        feedPath = []
        cabalsPath = []
        stocksPath = []
        profilePath = []
    }

    func path(for tab: MainTab) -> [AnyAppRoute] {
        switch tab {
        case .home: homePath
        case .feed: feedPath
        case .cabals: cabalsPath
        case .stocks: stocksPath
        case .profile: profilePath
        }
    }

    func open(_ route: some AppRoute, in tab: MainTab) {
        selectedTab = tab
        let path = binding(for: tab)
        let wrapped = AnyAppRoute(route)
        guard path.wrappedValue.last != wrapped else { return }
        path.wrappedValue.append(wrapped)
    }

    func open(cabalID: String, then route: some AppRoute, in tab: MainTab) {
        open(chain: [CabalRoute(id: cabalID), route], in: tab)
    }

    func openTransaction(cabalID: String, transactionID: String, in tab: MainTab) {
        open(cabalID: cabalID, then: TransactionRoute(cabalID: cabalID, transactionID: transactionID), in: tab)
    }

    func open(chain: [any AppRoute], in tab: MainTab) {
        guard let first = chain.first else { return }
        selectedTab = tab
        let path = binding(for: tab)
        var rest = chain.dropFirst().map { AnyAppRoute($0) }
        if let open = path.wrappedValue.lastIndex(of: AnyAppRoute(first)) {
            path.wrappedValue.removeSubrange((open + 1)...)
        } else {
            rest.insert(AnyAppRoute(first), at: 0)
        }
        path.wrappedValue.append(contentsOf: rest)
    }

    func select(_ tab: MainTab) {
        if tab == selectedTab {
            binding(for: tab).wrappedValue = []
        } else {
            selectedTab = tab
        }
    }

    func closeProposeFlow(in tab: MainTab) {
        let path = binding(for: tab)
        guard
            let start = path.wrappedValue.lastIndex(where: {
                $0.is(ProposeRoute.self) || $0.is(ProposeFromAssetRoute.self)
            })
        else { return }
        path.wrappedValue.removeSubrange(start...)
    }

    func closeCabal(id: String, in tab: MainTab) {
        let path = binding(for: tab)
        guard let cabal = path.wrappedValue.lastIndex(of: AnyAppRoute(CabalRoute(id: id))) else { return }
        path.wrappedValue.removeSubrange(cabal...)
    }

    func binding(for tab: MainTab) -> Binding<[AnyAppRoute]> {
        switch tab {
        case .home:
            Binding(get: { self.homePath }, set: { self.homePath = $0 })
        case .feed:
            Binding(get: { self.feedPath }, set: { self.feedPath = $0 })
        case .cabals:
            Binding(get: { self.cabalsPath }, set: { self.cabalsPath = $0 })
        case .stocks:
            Binding(get: { self.stocksPath }, set: { self.stocksPath = $0 })
        case .profile:
            Binding(get: { self.profilePath }, set: { self.profilePath = $0 })
        }
    }
}
