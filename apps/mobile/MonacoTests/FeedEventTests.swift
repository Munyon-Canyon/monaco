import Foundation
import MonacoAPI
import MonacoCore
import Testing

@testable import Monaco

@MainActor
struct FeedEventTests {
    private typealias Item = Components.Schemas.FeedItem

    private func loadedItems() async -> [Item] {
        let model = FeedModel.preview(.items, clock: ContinuousClock())
        await model.load()
        return model.items
    }

    private func pill(_ item: Item, figures: FeedFigures?) -> String {
        let event = FeedEvent(item, figures: figures)
        return "\(event.pillLabel)/\(event.pillTone)"
    }

    @Test func everyCardVariantNamesItselfWithItsOwnPill() async {
        let figures = FeedModel.previewFigures(.items)
        let items = await loadedItems()
        let pills = items.map { pill($0, figures: figures[$0.id]) }
        #expect(
            pills == [
                "Proposal/positive", "Sell/negative", "Price move/warn", "Discussion/discussion",
                "Propose sell/negative", "Approved/warn", "Cabal/positive",
            ])
    }

    @Test func aLiveTradeWithNoFiguresDefaultsToBuy() async throws {
        let items = await loadedItems()
        let sold = try #require(items.first { $0.kind == FeedKind.trade.rawValue })
        #expect(FeedEvent(sold, figures: nil) == .trade(.buy))
    }

    @Test func aKindFromANewerBackendIsAPlainUpdate() async throws {
        let items = await loadedItems()
        var item = try #require(items.first)
        item.kind = "something_new"
        let event = FeedEvent(item, figures: nil)
        #expect(event == .update)
        #expect(event.block == .quote)
        #expect(event.pillLabel == "Update")
    }

    @Test func blocksFollowTheEvent() {
        #expect(FeedEvent.proposal(.sell).block == .asset(.sell))
        #expect(FeedEvent.trade(.buy).block == .asset(.buy))
        #expect(FeedEvent.outcome(.buy).block == .asset(.buy))
        #expect(FeedEvent.priceMove.block == .price)
        #expect(FeedEvent.cabal.block == .cabal)
        #expect(FeedEvent.discussion.block == .quote)
    }

    @Test func aLiveTradeReadsItsDetailAndIsExecuted() async throws {
        let items = await loadedItems()
        let item = try #require(items.first { $0.kind == FeedKind.trade.rawValue })
        let facts = FeedAssetFacts(item, figures: nil, event: FeedEvent(item, figures: nil))
        #expect(facts.ticker == "AAPL")
        #expect(facts.detail == "Filled at $240.00")
        #expect(facts.status == "Executed")
        #expect(facts.amount == nil)
        #expect(facts.company == nil)
    }

    @Test func aLiveProposalReadsItsStatusFromTheRenderedDetail() async throws {
        let items = await loadedItems()
        let item = try #require(items.first { $0.kind == FeedKind.proposal.rawValue })
        let facts = FeedAssetFacts(item, figures: nil, event: FeedEvent(item, figures: nil))
        #expect(facts.status == "Voting open")
        #expect(facts.detail == nil)
        #expect(facts.thesis?.hasPrefix("A small position") == true)
    }

    @Test func fixtureFiguresFillTheBlockWithoutTouchingTheItem() async throws {
        let items = await loadedItems()
        let figures = FeedModel.previewFigures(.items)
        let item = try #require(items.first { $0.kind == FeedKind.trade.rawValue })
        let facts = FeedAssetFacts(
            item, figures: figures[item.id], event: FeedEvent(item, figures: figures[item.id]))
        #expect(facts.amount == "$480.00")
        #expect(facts.detail == "Sold 2 units · Avg $240.00")
        #expect(facts.company == "Apple")
    }

    @Test func aPriceMoveShowsTheChangeTheTitleStates() async throws {
        let items = await loadedItems()
        var item = try #require(items.first { $0.kind == FeedKind.priceMove.rawValue })
        item.title = "NVIDIA is up 4.18% today"
        item.tone = "positive"
        #expect(FeedPriceFacts.change(in: item) == "+4.18%")
        item.title = "Tesla is down 6.2% today"
        item.tone = "negative"
        #expect(FeedPriceFacts.change(in: item) == "\u{2212}6.2%")
        item.title = "Tesla is flat today"
        item.tone = "neutral"
        #expect(FeedPriceFacts.change(in: item) == nil)
        #expect(FeedPriceFacts(item, figures: nil).volume == nil)
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
