import Testing

@testable import Monaco

@MainActor
struct HomeSlotsTests {
    @Test func theHeroCabalsAndBoardSlotsAreLive() {
        #expect(HomePortfolioSlot.isLive)
        #expect(HomeCabalsSlot.isLive)
        #expect(HomePeopleBoardSlot.isLive)
    }

    @Test func homeKeepsItsSectionOrder() {
        let order = HomeScreen.sections.map { ObjectIdentifier($0) }
        let expected: [any HomeSection.Type] = [
            HomeNudgeSlot.self,
            HomePortfolioSlot.self,
            HomeBalanceSlot.self,
            HomePendingVotesSlot.self,
            HomeCabalsSlot.self,
            HomePeopleBoardSlot.self,
        ]
        #expect(order == expected.map { ObjectIdentifier($0) })
    }

    @Test func aCabalRowCountsItsMembers() {
        #expect(CabalPortfolioRow.members(1) == "1 member")
        #expect(CabalPortfolioRow.members(4) == "4 members")
    }
}
