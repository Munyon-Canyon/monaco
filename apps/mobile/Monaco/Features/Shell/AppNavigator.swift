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
        let wrapped = AnyAppRoute(route)
        switch tab {
        case .home: homePath.append(wrapped)
        case .feed: feedPath.append(wrapped)
        case .cabals: cabalsPath.append(wrapped)
        case .stocks: stocksPath.append(wrapped)
        case .profile: profilePath.append(wrapped)
        }
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
