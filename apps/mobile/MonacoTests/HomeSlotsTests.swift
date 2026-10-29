import MonacoAPI
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
            HomeFirstUseSlot.self,
            HomeCabalsSlot.self,
            HomeLeaderboardSlot.self,
            HomeEveryonePreviewSlot.self,
            HomeFeedPreviewSlot.self,
        ]
        #expect(order == expected.map { ObjectIdentifier($0) })
    }

    @Test func theLeaderboardRowOpensThePeopleBoardForTheMonth() {
        #expect(HomeLeaderboardSlot.route == LeaderboardRoute(board: .people, range: .oneMonth))
    }

    @Test func firstUseSwapsTheVoteChartAndCabalsListForTheEmptyStateAndEveryone() {
        let none = PortfolioSummary(.sampleEmpty)
        let live: [HomeBlock] = HomeBlock.allCases.filter { HomeFirstUse.shows($0, summary: none) }
        #expect(live == [.firstUse, .everyoneFeed])
        #expect(HomeFirstUseSlot.isLive)
        #expect(HomeEveryonePreviewSlot.isLive)
    }

    @Test func aMemberNeverSeesFirstUse() {
        let member = PortfolioSummary(.sample)
        let live: [HomeBlock] = HomeBlock.allCases.filter { HomeFirstUse.shows($0, summary: member) }
        #expect(live == [.vote, .chart, .cabalsList, .cabalsFeed])
    }

    @Test func firstUseDrawsDepositQuiet() {
        #expect(HomeScreen.cashEmphasis(for: PortfolioSummary(.sampleEmpty)) == .quiet)
        #expect(HomeScreen.cashEmphasis(for: PortfolioSummary(.sample)) == .prominent)
        #expect(HomeScreen.cashEmphasis(for: nil) == .prominent)
    }
}
