import MonacoCore
import SwiftUI
import Testing

@testable import Monaco

@MainActor
struct ScreenMapOrderTests {
    @Test func homeFollowsTheScreenMap() {
        #expect(
            names(HomeScreen.sections) == [
                "HomeFigureSlot", "HomePendingVotesSlot", "HomeChartSlot", "HomeBalanceSlot", "HomeCabalsSlot",
                "HomeLeaderboardSlot",
            ])
    }

    @Test func cabalsTabFollowsTheScreenMap() {
        #expect(
            names(CabalsTabScreen.sections) == [
                "CabalsJoinSlot", "CabalsVoteSlot", "CabalsGridSlot", "CabalsShareListSlot",
            ])
    }

    @Test func cabalScreenFollowsTheScreenMap() {
        #expect(
            names(CabalScreen.sections) == [
                "CabalHeaderSlot", "CabalClosedSlot", "CabalPauseSlot", "CabalJoinSlot", "CabalPotSlot",
                "CabalSliceSlot",
                "CabalActionsSlot", "CabalEmptySlot", "CabalStartersSlot", "CabalProposalsSlot",
                "CabalTabs",
            ])
    }

    @Test func cabalInformationFollowsTheScreenMap() {
        #expect(
            names(CabalScreen.detailsSections) == [
                "CabalInfoCoverSlot", "CabalRulesSlot", "CabalInfoLinksSlot", "CabalTreasurySlot",
                "CabalInfoFooterSlot", "CabalLeaveSlot",
            ])
    }

    @Test func profileFollowsTheScreenMap() {
        #expect(
            names(ProfileScreen.sections) == [
                "ProfileCardSlot", "ProfileStandingSlot", "ProfileCashSlot", "ProfileTabsSlot",
            ])
    }

    @Test func proposeFromAssetRouteRendersItsScreen() {
        let view: Any = ProposeFromAssetRoute(symbol: "GOOGLx", kind: .buy).destination()
        #expect(view is ProposeFromAssetScreen)
    }

    @Test func theFundRouteRendersItsScreen() {
        let view: Any = FundRoute(cabalID: "c").destination()
        #expect(view is FundCabalScreen)
    }

    @Test func theChatRouteRendersItsScreen() {
        let view: Any = ChatRoute(cabalID: "c").destination()
        #expect(view is GroupChatView)
    }

    @Test func theActivityRouteRendersItsScreen() {
        let view: Any = AccountActivityRoute().destination()
        #expect(view is AccountActivityView)
    }

    @Test func theDeleteAccountRouteRendersItsScreen() {
        let view: Any = DeleteAccountRoute().destination()
        #expect(view is DeleteAccountView)
    }

    private func names<T>(_ sections: [T]) -> [String] {
        sections.map { String(describing: $0) }
    }
}
