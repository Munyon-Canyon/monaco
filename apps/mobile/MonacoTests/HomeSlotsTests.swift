import Foundation
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

    @Test func firstUseDrawsItsEmptyStateAndTheEveryoneFeed() {
        #expect(HomeFirstUseSlot.isLive)
        #expect(HomeEveryonePreviewSlot.isLive)
    }

    @Test func firstUseGateFollowsTheEmptyPortfolio() async {
        let empty = PortfolioModel.preview(.sampleEmpty)
        let funded = PortfolioModel.preview(.sample)
        await empty.load()
        await funded.load()
        #expect(HomeGate(portfolio: empty, reads: nil, balance: .idle).firstUse)
        #expect(!HomeGate(portfolio: funded, reads: nil, balance: .idle).firstUse)
        #expect(!HomeGate(portfolio: nil, reads: nil, balance: .idle).firstUse)
    }

    @Test func joinLinkAcceptsOnlyMonacoInviteLinks() throws {
        #expect(JoinLinkCopy.accepts(try #require(URL(string: "https://monacolabs.xyz/r/abc123"))))
        #expect(!JoinLinkCopy.accepts(try #require(URL(string: "https://example.com/r/abc123"))))
    }

    @Test func theLeaderboardRowOpensThePeopleBoardForTheMonth() {
        #expect(HomeLeaderboardSlot.route == LeaderboardRoute(board: .people, range: .oneMonth))
    }

    @Test func aSettledVoteCardCollapsesOnlyAfterItsFirstSecond() {
        let model = HomeVoteCardModel(
            api: APIClient(
                serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"),
                transport: StubTransport(.json(.ok, "[]"))),
            hints: FakeHintSource(),
            row: HomePendingVote(
                id: "p1", cabalID: "c1", cabalName: "AC Capital", kind: "buy", symbol: "NVDAx", usdcMicros: 400_000_000,
                tally: ProposalTally(yes: 4, no: 1, voters: 8, needed: 5), replyCount: 6,
                expiresAt: .now.addingTimeInterval(3_600)))
        let tally = ProposalTally(yes: 5, no: 1, voters: 8, needed: 5)
        let myVote = ProposalMyVote(order: 5, of: 8, yesOrder: 5, noOrder: 0, decided: true)
        let open = CastOutcome(choice: .yes, status: .open, tally: tally, undoUntil: nil, myVote: myVote)
        let passed = CastOutcome(choice: .yes, status: .passed, tally: tally, undoUntil: nil, myVote: myVote)
        let now: Date = .now
        #expect(!HomeVoteCard.hasCollapsed(model, now: now))
        model.preview(.committed(open))
        #expect(!HomeVoteCard.hasCollapsed(model, now: now.addingTimeInterval(60)))
        model.preview(.committed(passed))
        #expect(!HomeVoteCard.hasCollapsed(model, now: .now))
        #expect(HomeVoteCard.hasCollapsed(model, now: .now.addingTimeInterval(HomeVoteCard.collapseDelay + 1)))
    }
}
