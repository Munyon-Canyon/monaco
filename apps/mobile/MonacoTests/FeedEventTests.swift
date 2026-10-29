import Foundation
import MonacoAPI
import MonacoCore
import Testing

@testable import Monaco

struct FeedEventTests {
    private typealias Item = Components.Schemas.FeedItem

    @Test func aPostDrawsTheLogoTheFeedSends() {
        var item = Item.samples[0]
        item.assetLogoUrl = " https://cdn.example/AAPLx.png "
        #expect(item.assetLogoURL == URL(string: "https://cdn.example/AAPLx.png"))
        item.assetLogoUrl = nil
        #expect(item.assetLogoURL == nil)
        item.assetLogoUrl = ""
        #expect(item.assetLogoURL == nil)
    }

    @Test func thePostColumnStartsWhereTheGutterAvatarAndGapEnd() {
        #expect(FeedPostMetrics.gutter == MonacoTheme.Space.gutter)
        #expect(FeedPostMetrics.lead == MonacoAvatarSize.feed.rawValue)
        #expect(FeedPostMetrics.textColumn == 60)
    }

    @Test func aDestinationMapsToItsOneRoute() {
        #expect(FeedDestination.proposal(proposalID: "p1").route == AnyAppRoute(ProposalRoute(proposalID: "p1")))
        #expect(FeedDestination.post(itemID: "i1").route == AnyAppRoute(FeedItemRoute(itemID: "i1")))
    }

    @Test func aProposalAndAJoinOpenWhereTheRuleSays() {
        let proposal = Item.samples[0]
        #expect(proposal.opensRoute == AnyAppRoute(ProposalRoute(proposalID: proposal.refId)))
        let join = Item.samples[4]
        #expect(join.opensRoute == AnyAppRoute(CabalRoute(id: Item.sampleCabalID)))
    }

    @Test func aReturnReadsAsOneMonthAndAHiddenOneAsPrivate() throws {
        let shown = try #require(FeedReturnFacts(.percent("+8.2%")))
        #expect(shown.text == "+8.2% · 1M")
        #expect(shown.spoken == "Personal return up 8.2 percent for one month")
        #expect(!shown.isPrivate)
        let hidden = try #require(FeedReturnFacts(.hidden))
        #expect(hidden.text == "P&L private")
        #expect(hidden.spoken == "Personal performance private")
        #expect(hidden.isPrivate)
        #expect(FeedReturnFacts(nil) == nil)
    }
}

@MainActor
struct FeedPostPhaseTests {
    @Test func aPostLoadsThroughLoadingToReady() async {
        let model = FeedItemDetailModel.preview(.ready, clock: ContinuousClock())
        #expect(FeedPostPhase(model) == .loading)
        await model.load()
        guard case .ready(let item) = FeedPostPhase(model) else {
            Issue.record("the loaded post is not ready")
            return
        }
        #expect(item.id == model.itemID)
    }

    @Test func aMissingOrForbiddenPostIsUnavailable() async {
        let model = FeedItemDetailModel.preview(.unavailable, clock: ContinuousClock())
        await model.load()
        #expect(FeedPostPhase(model) == .unavailable)
    }

    @Test func anyOtherFailureIsATryAgain() async {
        let transport = StubTransport(scripted: [.failure(URLError(.notConnectedToInternet))])
        let model = FeedItemDetailModel(
            itemID: "item-1",
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            hints: FakeHintSource(), clock: ContinuousClock())
        await model.load()
        #expect(FeedPostPhase(model) == .failed)
    }
}
