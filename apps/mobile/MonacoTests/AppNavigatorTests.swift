import SwiftUI
import Testing

@testable import Monaco

@MainActor
struct AppNavigatorTests {
    @Test func openSelectsCabalsAndAppendsOnlyThatPath() {
        let navigator = AppNavigator()

        navigator.open(ProbeRoute(marker: "cabal"), in: .cabals)

        #expect(navigator.selectedTab == .cabals)
        #expect(navigator.path(for: .cabals).count == 1)
        for tab in MainTab.allCases where tab != .cabals {
            #expect(navigator.path(for: tab).isEmpty)
        }
    }

    @Test func closingTheProposeFlowPopsToTheScreenBeforeIt() {
        let navigator = AppNavigator()
        navigator.open(ProbeRoute(marker: "cabal"), in: .cabals)
        navigator.open(ProposeRoute(cabalID: "cabal"), in: .cabals)

        navigator.closeProposeFlow()

        #expect(navigator.path(for: .cabals).count == 1)
    }
}

nonisolated private struct ProbeRoute: AppRoute {
    let marker: String

    func destination() -> some View {
        Text(marker)
    }
}
