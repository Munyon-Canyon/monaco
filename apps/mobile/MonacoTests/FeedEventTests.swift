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

    private func pill(_ item: Item) -> String {
        let event = FeedEvent(item)
        return "\(event.pillLabel)/\(event.pillTone)"
    }

    @Test func everyCardVariantNamesItselfWithItsOwnPill() async {
        let items = await loadedItems()
        #expect(
            items.map(pill) == [
                "Proposal/positive", "Sell/negative", "Price move/warn", "Discussion/discussion",
                "Propose sell/negative", "Approved/warn", "Cabal/positive",
            ])
    }

    @Test func aTradeWithNoActionDefaultsToBuy() async throws {
        let items = await loadedItems()
        var item = try #require(items.first { $0.kind == FeedKind.trade.rawValue })
        item.action = nil
        #expect(FeedEvent(item) == .trade(.buy))
    }

    @Test func aKindFromANewerBackendIsAPlainUpdate() async throws {
        let items = await loadedItems()
        var item = try #require(items.first)
        item.kind = "something_new"
        let event = FeedEvent(item)
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

    @Test func aTradeFillsTheBlockFromTheWireFigures() async throws {
        let items = await loadedItems()
        let item = try #require(items.first { $0.kind == FeedKind.trade.rawValue })
        let facts = FeedAssetFacts(item, event: FeedEvent(item))
        #expect(facts.ticker == "AAPL")
        #expect(facts.amount == "$480.00")
        #expect(facts.detail == "Sold 2 units · Avg $240.00")
        #expect(facts.company == "Apple")
        #expect(facts.status == "Executed")
    }

    @Test func aTradeWithNoFiguresFallsBackToTheRenderedDetail() async throws {
        let items = await loadedItems()
        var item = try #require(items.first { $0.kind == FeedKind.trade.rawValue })
        item.usdcMicros = nil
        item.tokenAmount = nil
        item.tokenDecimals = nil
        item.priceMicros = nil
        item.assetName = nil
        let facts = FeedAssetFacts(item, event: FeedEvent(item))
        #expect(facts.detail == "Filled at $240.00")
        #expect(facts.amount == nil)
        #expect(facts.company == nil)
    }

    @Test func aProposalReadsItsShareOfThePotAndItsStatus() async throws {
        let items = await loadedItems()
        let item = try #require(items.first { $0.kind == FeedKind.proposal.rawValue })
        let facts = FeedAssetFacts(item, event: FeedEvent(item))
        #expect(facts.status == "Voting open")
        #expect(facts.amount == "$400.00")
        #expect(facts.detail == "2.00% of Big Ballers’s pot")
        #expect(facts.thesis?.hasPrefix("A small position") == true)
    }

    @Test func aProposalMadeBeforeTheShareShippedHasNoShareLine() async throws {
        let items = await loadedItems()
        var item = try #require(items.first { $0.kind == FeedKind.proposal.rawValue })
        item.shareOfPotBps = nil
        #expect(FeedAssetFacts(item, event: FeedEvent(item)).detail == nil)
    }

    @Test func aPriceMoveShowsTheChangeTheWireStates() async throws {
        let items = await loadedItems()
        var item = try #require(items.first { $0.kind == FeedKind.priceMove.rawValue })
        #expect(FeedPriceFacts(item).change == "+4.18%")
        #expect(FeedPriceFacts(item).company == "NVIDIA")
        item.changeBps = -620
        #expect(FeedPriceFacts(item).change == "\u{2212}6.20%")
        item.changeBps = nil
        #expect(FeedPriceFacts(item).change == nil)
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
