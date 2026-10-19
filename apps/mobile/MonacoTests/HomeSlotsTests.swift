import Testing

@testable import Monaco

@MainActor
struct HomeSlotsTests {
    @Test func theHeroCabalsAndBoardSlotsAreLive() {
        #expect(HomePortfolioSlot.isLive)
        #expect(HomeCabalsSlot.isLive)
        #expect(HomeLeaderboardSlot.isLive)
        #expect(HomeMarketUpdatesSlot.isLive)
    }

    @Test func homeKeepsItsSectionOrder() {
        let order = HomeScreen.sections.map { ObjectIdentifier($0) }
        let expected: [any HomeSection.Type] = [
            HomeNudgeSlot.self,
            HomePortfolioSlot.self,
            HomeBalanceSlot.self,
            HomePendingVotesSlot.self,
            HomeCabalsSlot.self,
            HomeLeaderboardSlot.self,
            HomeMarketUpdatesSlot.self,
        ]
        #expect(order == expected.map { ObjectIdentifier($0) })
    }
}
