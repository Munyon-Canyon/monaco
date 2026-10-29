import MonacoCore
import MonacoRanking
import Testing

@testable import Monaco

struct CallBoardRouteTests {
    @Test func theRouteOpensOnAGivenTabAndPeriod() {
        let route = LeaderboardRoute(tab: .people, period: .week)

        #expect(route.tab == .people)
        #expect(route.period == .week)
        #expect(LeaderboardRoute() == LeaderboardRoute(tab: .people, period: .month))
    }

    @Test func theOldInitMapsRangesAndBoards() {
        #expect(LeaderboardRoute(board: .cabals, range: .oneWeek) == LeaderboardRoute(tab: .cabals, period: .week))
        #expect(LeaderboardRoute(board: .people, range: .oneMonth) == LeaderboardRoute(tab: .people, period: .month))
        #expect(LeaderboardRoute(board: .people, range: .all) == LeaderboardRoute(tab: .people, period: .all))
        #expect(LeaderboardRoute(board: .cabals, range: .threeMonths) == LeaderboardRoute(tab: .cabals, period: .all))
    }
}
