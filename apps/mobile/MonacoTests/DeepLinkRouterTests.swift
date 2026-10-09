import Foundation
import SwiftUI
import Testing

@testable import Monaco

@MainActor
struct DeepLinkRouterTests {
    @Test func matchingURLOpensItsRouteInItsTab() async {
        let navigator = AppNavigator()

        await DeepLinkRouter.handle(
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

    @Test func matchingURLClosesOpenSheetsBeforeItsRouteOpens() async {
        let navigator = AppNavigator()
        var routeWasOpenWhenSheetsClosed: [Bool] = []

        await DeepLinkRouter.handle(
            URL(string: "monaco://match")!,
            navigator: navigator,
            handlers: [ProbeDeepLink.self],
            dismissPresented: { routeWasOpenWhenSheetsClosed.append(!navigator.path(for: .cabals).isEmpty) }
        )

        #expect(routeWasOpenWhenSheetsClosed == [false])
        #expect(navigator.path(for: .cabals).count == 1)
    }

    @Test func nonMatchingURLLeavesOpenSheetsAlone() async {
        let navigator = AppNavigator()
        var closings = 0

        await DeepLinkRouter.handle(
            URL(string: "monaco://nothing")!,
            navigator: navigator,
            handlers: [NeverMatchesDeepLink.self],
            dismissPresented: { closings += 1 }
        )

        #expect(closings == 0)
    }

    @Test func nonMatchingURLOpensNothing() async {
        let navigator = AppNavigator()

        await DeepLinkRouter.handle(
            URL(string: "monaco://nothing")!,
            navigator: navigator,
            handlers: [NeverMatchesDeepLink.self]
        )

        #expect(navigator.selectedTab == .home)
        for tab in MainTab.allCases {
            #expect(navigator.path(for: tab).isEmpty)
        }
    }

    @Test func twoMatchingHandlersOnlyOpenTheFirst() async {
        let navigator = AppNavigator()

        await DeepLinkRouter.handle(
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
