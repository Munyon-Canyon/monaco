import SwiftUI

protocol AppRoute: Hashable, Sendable {
    associatedtype Destination: View
    @MainActor @ViewBuilder func destination() -> Destination
}

struct AnyAppRoute: Hashable, Sendable {
    private let route: any AppRoute

    init(_ route: some AppRoute) {
        self.route = route
    }

    static func == (lhs: Self, rhs: Self) -> Bool {
        AnyHashable(lhs.route) == AnyHashable(rhs.route)
    }

    func hash(into hasher: inout Hasher) {
        AnyHashable(route).hash(into: &hasher)
    }

    func `is`<Route: AppRoute>(_ type: Route.Type) -> Bool { route is Route }

    @MainActor func destination() -> AnyView {
        AnyView(route.destination())
    }
}
