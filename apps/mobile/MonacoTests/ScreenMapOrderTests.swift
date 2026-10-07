import SwiftUI
import Testing

@testable import Monaco

@MainActor
struct ScreenMapOrderTests {
    @Test func homeFollowsTheScreenMap() {
        #expect(
            names(HomeScreen.sections) == [
                "HomeNudgeSlot", "HomePortfolioSlot", "HomeBalanceSlot", "HomePendingVotesSlot", "HomeCabalsSlot",
                "HomePeopleBoardSlot",
            ])
    }

    @Test func cabalsTabFollowsTheScreenMap() {
        #expect(
            names(CabalsTabScreen.sections) == [
                "CabalsJoinSlot", "CabalsInvitesSlot", "CabalsListSlot", "CabalsValueChartSlot", "CabalsBoardSlot",
            ])
    }

    @Test func aTypedSearchHidesEverythingBelowIt() {
        #expect(names(CabalsTabScreen.visible(CabalsTabScreen.sections, searching: true)) == ["CabalsJoinSlot"])
        #expect(names(CabalsTabScreen.visible(CabalsTabScreen.sections, searching: false)).count == 5)
    }

    @Test func cabalScreenFollowsTheScreenMap() {
        #expect(
            names(CabalScreen.sections) == [
                "CabalHeaderSlot", "CabalPotSlot", "CabalValueChartSlot", "CabalSliceSlot", "CabalPauseSlot",
                "CabalJoinSlot", "CabalActionsSlot", "CabalProposalsSlot", "CabalHoldingsSlot", "CabalAgentSlot",
                "CabalMemberBoardSlot", "CabalActivitySlot",
            ])
    }

    @Test func cabalDetailsSheetFollowsTheScreenMap() {
        #expect(
            names(CabalScreen.detailsSections) == [
                "CabalInviteCodeSlot", "CabalInviteMemberSlot", "CabalRulesSlot", "CabalTreasurySlot", "CabalLeaveSlot",
            ])
    }

    @Test func profileFollowsTheScreenMap() {
        #expect(
            names(ProfileScreen.sections) == [
                "ProfileHeaderSlot", "ProfileFollowCountsSlot", "ProfileStatsSlot", "ProfileBalanceSlot",
                "ProfileCabalsSlot", "ProfileInviteSlot", "ProfileFindFriendsSlot", "ProfileSettingsSlot",
            ])
    }

    @Test func routeStubsRenderTheirPlaceholder() {
        let stubs: [(Any, String)] = [(AgentRoute(cabalID: "c").destination(), "Trading bot")]
        for (view, screen) in stubs {
            #expect((view as? NotMigratedView)?.screen == screen)
        }
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
