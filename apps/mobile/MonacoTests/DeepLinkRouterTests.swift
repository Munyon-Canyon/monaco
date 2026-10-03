import Foundation
import SwiftUI
import Testing

@testable import Monaco

@MainActor
struct DeepLinkRouterTests {
    @Test func matchingURLOpensItsRouteInItsTab() {
        let navigator = AppNavigator()

        DeepLinkRouter.handle(
            URL(string: "monaco://match")!,
            navigator: navigator,
            handlers: [ProbeDeepLink.self, NeverMatchesDeepLink.self]
        )

        #expect(navigator.selectedTab == .cabals)
        #expect(navigator.path(for: .cabals).count == 1)
        for tab in MainTab.allCases where tab != .cabals {
            #expect(navigator.path(for: tab).isEmpty)
        }
    }

    @Test func nonMatchingURLOpensNothing() {
        let navigator = AppNavigator()

        DeepLinkRouter.handle(
            URL(string: "monaco://nothing")!,
            navigator: navigator,
            handlers: [NeverMatchesDeepLink.self]
        )

        #expect(navigator.selectedTab == .home)
        for tab in MainTab.allCases {
            #expect(navigator.path(for: tab).isEmpty)
        }
    }

    @Test func twoMatchingHandlersOnlyOpenTheFirst() {
        let navigator = AppNavigator()

        DeepLinkRouter.handle(
            URL(string: "monaco://either")!,
            navigator: navigator,
            handlers: [AlwaysMatchesInCabals.self, AlwaysMatchesInProfile.self]
        )

        #expect(navigator.selectedTab == .cabals)
        #expect(navigator.path(for: .cabals).count == 1)
        #expect(navigator.path(for: .profile).isEmpty)
    }
}

nonisolated private struct ProbeRoute: AppRoute {
    let marker: String

    func destination() -> some View {
        Text(marker)
    }
}

private enum ProbeDeepLink: DeepLinkHandler {
    static func route(for url: URL) -> (any AppRoute, MainTab)? {
        guard url.host == "match" else { return nil }
        return (ProbeRoute(marker: "matched"), .cabals)
    }
}

private enum NeverMatchesDeepLink: DeepLinkHandler {
    static func route(for url: URL) -> (any AppRoute, MainTab)? {
        nil
    }
}

private enum AlwaysMatchesInCabals: DeepLinkHandler {
    static func route(for url: URL) -> (any AppRoute, MainTab)? {
        (ProbeRoute(marker: "cabals"), .cabals)
    }
}

private enum AlwaysMatchesInProfile: DeepLinkHandler {
    static func route(for url: URL) -> (any AppRoute, MainTab)? {
        (ProbeRoute(marker: "profile"), .profile)
    }
}
