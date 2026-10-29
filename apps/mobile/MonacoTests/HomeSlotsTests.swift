import MonacoCore
import Testing

@testable import Monaco

@MainActor
struct HomeSlotsTests {
    @Test func theFigureChartCabalsAndBoardSlotsAreLive() {
        #expect(HomeFigureSlot.isLive)
        #expect(HomeChartSlot.isLive)
        #expect(HomeCabalsSlot.isLive)
        #expect(HomeLeaderboardSlot.isLive)
    }

    @Test func homeDrawsNoMarketUpdatesAndNoNudge() {
        let names = HomeScreen.sections.map { String(describing: $0) }
        #expect(!names.contains("HomeMarketUpdatesSlot"))
        #expect(!names.contains("HomeNudgeSlot"))
    }

    @Test func homeKeepsItsSectionOrder() {
        let order = HomeScreen.sections.map { ObjectIdentifier($0) }
        let expected: [any HomeSection.Type] = [
            HomeFigureSlot.self,
            HomePendingVotesSlot.self,
            HomeChartSlot.self,
            HomeBalanceSlot.self,
            HomeCabalsSlot.self,
            HomeLeaderboardSlot.self,
            HomeFeedPreviewSlot.self,
        ]
        #expect(order == expected.map { ObjectIdentifier($0) })
    }

    @Test func theLeaderboardRowOpensThePeopleBoardForTheMonth() {
        #expect(HomeLeaderboardSlot.route == LeaderboardRoute(board: .people, range: .oneMonth))
    }
}
