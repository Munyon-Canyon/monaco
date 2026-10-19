import MonacoCore
import SwiftUI
import Testing

@testable import Monaco

@MainActor
struct ScreenMapOrderTests {
    @Test func homeFollowsTheScreenMap() {
        #expect(
            names(HomeScreen.sections) == [
                "HomeNudgeSlot", "HomePortfolioSlot", "HomeBalanceSlot", "HomePendingVotesSlot", "HomeCabalsSlot",
                "HomeLeaderboardSlot", "HomeMarketUpdatesSlot",
            ])
    }

    @Test func cabalsTabFollowsTheScreenMap() {
        #expect(
            names(CabalsTabScreen.sections) == [
                "CabalsJoinSlot", "CabalsInvitesSlot", "CabalsListSlot", "CabalsBoardSlot",
            ])
    }

    @Test func cabalScreenFollowsTheScreenMap() {
        #expect(
            names(CabalScreen.sections) == [
                "CabalHeaderSlot", "CabalClosedSlot", "CabalPotSlot", "CabalValueChartSlot", "CabalSliceSlot",
                "CabalPauseSlot",
                "CabalJoinSlot", "CabalActionsSlot", "CabalProposalsSlot", "CabalHoldingsSlot",
                "CabalMemberBoardSlot", "CabalActivitySlot",
            ])
    }

    @Test func cabalDetailsSheetFollowsTheScreenMap() {
        #expect(
            names(CabalScreen.detailsSections) == [
                "CabalInviteMemberSlot", "CabalRulesSlot", "CabalTreasurySlot", "CabalLeaveSlot",
            ])
    }

    @Test func profileFollowsTheScreenMap() {
        #expect(
            names(ProfileScreen.sections) == [
                "ProfileHeaderSlot", "ProfileFollowCountsSlot", "ProfileStatsSlot", "ProfileBalanceSlot",
                "ProfileCabalsSlot", "ProfileLinksSlot",
            ])
    }

    @Test func proposeFromAssetRouteRendersItsScreen() {
        let view: Any = ProposeFromAssetRoute(symbol: "GOOGLx", kind: .buy).destination()
        #expect(view is ProposeFromAssetScreen)
    }

    @Test func theFundRouteRendersItsScreen() {
        let view: Any = FundRoute(cabalID: "c").destination()
        #expect(view is FundCabalView)
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
