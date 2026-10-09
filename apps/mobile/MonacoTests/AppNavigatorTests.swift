import MonacoCore
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

        navigator.closeProposeFlow(in: .cabals)

        #expect(navigator.path(for: .cabals).count == 1)
    }

    @Test func closingTheProposeFlowTrimsItsOwnTabWhileAnotherIsSelected() {
        let navigator = AppNavigator()
        navigator.open(ProbeRoute(marker: "cabal"), in: .cabals)
        navigator.open(ProposeRoute(cabalID: "cabal"), in: .cabals)
        navigator.open(ProbeRoute(marker: "home"), in: .home)

        navigator.closeProposeFlow(in: .cabals)

        #expect(navigator.cabalsPath == [AnyAppRoute(ProbeRoute(marker: "cabal"))])
        #expect(navigator.homePath == [AnyAppRoute(ProbeRoute(marker: "home"))])
        #expect(navigator.selectedTab == .home)
    }

    @Test func closingTheProposeFlowLeavesAnotherTabsFlowOpen() {
        let navigator = AppNavigator()
        navigator.open(ProposeRoute(cabalID: "cabal"), in: .cabals)
        navigator.open(ProposeFromAssetRoute(symbol: "AAPL", kind: .buy), in: .stocks)
        navigator.selectedTab = .stocks

        navigator.closeProposeFlow(in: .cabals)

        #expect(navigator.cabalsPath.isEmpty)
        #expect(navigator.stocksPath == [AnyAppRoute(ProposeFromAssetRoute(symbol: "AAPL", kind: .buy))])
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

    @Test func aChainForACabalAlreadyOpenCutsBackToItInsteadOfStackingASecond() {
        let navigator = AppNavigator()
        navigator.open(CabalRoute(id: "c-1"), in: .cabals)
        navigator.open(ProbeRoute(marker: "chat"), in: .cabals)

        navigator.open(chain: [CabalRoute(id: "c-1"), ProposalRoute(proposalID: "p-1")], in: .cabals)

        #expect(
            navigator.cabalsPath == [AnyAppRoute(CabalRoute(id: "c-1")), AnyAppRoute(ProposalRoute(proposalID: "p-1"))])
    }

    @Test func tappingTheTabThatIsShowingPopsItToItsRoot() {
        let navigator = AppNavigator()
        navigator.open(ProbeRoute(marker: "cabal"), in: .cabals)

        navigator.select(.home)
        #expect(navigator.cabalsPath.count == 1)
        navigator.select(.cabals)
        #expect(navigator.cabalsPath.count == 1)
        navigator.select(.cabals)
        #expect(navigator.cabalsPath.isEmpty)
    }
}

nonisolated private struct ProbeRoute: AppRoute {
    let marker: String

    func destination() -> some View {
        Text(marker)
    }
}
