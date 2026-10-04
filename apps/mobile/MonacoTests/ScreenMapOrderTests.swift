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
                "CabalInviteCodeSlot", "CabalInviteMemberSlot", "CabalRulesSlot", "CabalTreasurySlot", "CabalEditSlot",
                "CabalLeaveSlot",
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
        let stubs: [(Any, String)] = [
            (FundRoute(cabalID: "c").destination(), "Fund this cabal"),
            (ChatRoute(cabalID: "c").destination(), "Chat"),
            (AgentRoute(cabalID: "c").destination(), "Trading bot"),
            (ProposeFromAssetRoute(symbol: "GOOGLx", kind: .buy).destination(), "Propose"),
        ]
        for (view, screen) in stubs {
            #expect((view as? NotMigratedView)?.screen == screen)
        }
    }

    @Test func theActivityRouteRendersItsScreen() {
        let view: Any = AccountActivityRoute().destination()
        #expect(view is AccountActivityView)
    }

    private func names<T>(_ sections: [T]) -> [String] {
        sections.map { String(describing: $0) }
    }
}
