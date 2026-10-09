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

    @Test func closingACabalPopsItAndWhatIsAboveKeepingWhatIsBelow() {
        let navigator = AppNavigator()
        navigator.open(ProbeRoute(marker: "below"), in: .home)
        navigator.open(CabalRoute(id: "cabal"), in: .home)
        navigator.open(ProbeRoute(marker: "above"), in: .home)
        navigator.open(ProbeRoute(marker: "other"), in: .cabals)
        navigator.selectedTab = .home

        navigator.closeCabal(id: "cabal", in: .home)

        #expect(navigator.homePath == [AnyAppRoute(ProbeRoute(marker: "below"))])
        #expect(navigator.cabalsPath.count == 1)
        #expect(navigator.selectedTab == .home)
    }

    @Test func closingACabalNotInThePathLeavesItUnchanged() {
        let navigator = AppNavigator()
        navigator.open(ProbeRoute(marker: "below"), in: .home)
        navigator.open(CabalRoute(id: "other"), in: .home)

        navigator.closeCabal(id: "cabal", in: .home)

        #expect(navigator.homePath.count == 2)
    }
}

nonisolated private struct ProbeRoute: AppRoute {
    let marker: String

    func destination() -> some View {
        Text(marker)
    }
}
