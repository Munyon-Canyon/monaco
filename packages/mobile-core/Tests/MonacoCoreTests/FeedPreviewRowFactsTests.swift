import Foundation
import MonacoAPI
import MonacoCore
import XCTest

final class FeedPreviewRowFactsTests: XCTestCase {
    private typealias Item = Components.Schemas.FeedItem
    private let now = Date(timeIntervalSince1970: 1_800_000_000)

    private func facts(
        _ item: Item, _ source: HomeFeedPreviewModel.Source = .yourCabals, ago hours: Double = 5
    ) -> FeedPreviewRowFacts {
        var aged = item
        aged.createdAt = now.addingTimeInterval(-hours * 3600)
        return FeedPreviewRowFacts(aged, source: source, now: now)
    }

    private func proposal(replies: Int32 = 6) -> Item {
        var item = Item.samples[0]
        item.symbol = "NVDAx"
        item.action = "buy"
        item.usdcMicros = 400_000_000
        item.cabalName = "AC Capital"
        item.replyCount = replies
        item.expiresAt = now.addingTimeInterval(4 * 3600 - 60)
        return item
    }

    func testAnOpenProposalReadsTheCabalThenTheOrderWithItsVoteLine() {
        let row = facts(proposal(), ago: 1)
        XCTAssertEqual(row.title, "AC Capital · Buy NVDA $400")
        XCTAssertEqual(row.subtitle, "5 of 8 voted · ends in 4h")
        XCTAssertEqual(row.trailing, .split(yes: 4, no: 1))
        XCTAssertEqual(row.replies, 6)
        XCTAssertEqual(row.destination, FeedDestination.destination(for: proposal()))
        guard case .person(let name, _, _) = row.lead else { return XCTFail("expected a person") }
        XCTAssertEqual(name, "Maya")
    }

    func testAnOpenProposalFromEveryoneLeadsWithTheOrder() {
        let row = facts(proposal(), .everyone, ago: 1)
        XCTAssertEqual(row.title, "Buy NVDA $400")
        XCTAssertEqual(row.subtitle, "Maya · AC Capital · 1h")
        XCTAssertEqual(row.trailing, .split(yes: 4, no: 1))
    }

    func testAProposalWithoutATallyHasNoSplit() {
        var item = proposal()
        item.tally = nil
        let row = facts(item)
        XCTAssertEqual(row.title, "Buy NVDA $400")
        XCTAssertEqual(row.subtitle, "AC Capital · 5h")
        XCTAssertNil(row.trailing)
    }

    func testAPassedSaleNamesTheCabalAndTheTopCallWhenItReturnedAGain() {
        let row = facts(Item.results[1])
        XCTAssertEqual(row.title, "Tech Titans sold AAPL")
        XCTAssertEqual(row.subtitleParts, ["Logan called the top", "5h"])
        XCTAssertEqual(row.trailing, .amount(dollars: "$480", bps: 670))
        XCTAssertEqual(row.trailing?.returnText, "+6.7%")
        XCTAssertEqual(row.replies, 5)
        guard case .asset(let symbol, _, _, let isPreIPO) = row.lead else { return XCTFail("expected the logo") }
        XCTAssertEqual(symbol, "AAPLx")
        XCTAssertFalse(isPreIPO)
        XCTAssertEqual(row.destination, .post(itemID: Item.results[1].id))
    }

    func testAPassedSaleAtALossIsTheirCallNotTheTop() {
        var item = Item.results[1]
        item.returnBps = -250
        let row = facts(item)
        XCTAssertEqual(row.subtitleParts, ["Logan’s call", "5h"])
        XCTAssertEqual(row.trailing?.returnText, "−2.5%")
    }

    func testAPassedSaleWithNoCallerHasOnlyTheAge() {
        var item = Item.results[1]
        item.caller = nil
        XCTAssertEqual(facts(item).subtitleParts, ["5h"])
    }

    func testAPassedSaleFromEveryoneLeadsWithTheOrderAndTrailsTheReturn() {
        let row = facts(Item.results[1], .everyone)
        XCTAssertEqual(row.title, "Logan sold AAPL $480")
        XCTAssertEqual(row.subtitle, "Tech Titans · 5h")
        XCTAssertEqual(row.trailing, .amount(dollars: nil, bps: 670))
    }

    func testAPassedBuyIsCalledItWithTheCallerAndTrailsTheTicker() {
        let row = facts(Item.results[0])
        XCTAssertEqual(row.title, "Christelle called it")
        XCTAssertEqual(row.subtitle, "Buy META · Degens · 5h")
        XCTAssertEqual(row.trailing, .symbol("META", bps: 109))
        XCTAssertEqual(row.trailing?.returnText, "+1.1%")
        guard case .person(let name, _, _) = row.lead else { return XCTFail("expected the caller") }
        XCTAssertEqual(name, "Christelle")
        XCTAssertEqual(facts(Item.results[0], .everyone).title, row.title)
    }

    func testAPassedBuyWithNoCallerNamesTheCabal() {
        var item = Item.results[0]
        item.caller = nil
        let row = facts(item)
        XCTAssertEqual(row.title, "Degens bought META")
        guard case .cabal(let name, _, _) = row.lead else { return XCTFail("expected the cabal") }
        XCTAssertEqual(name, "Degens")
    }

    func testAPreIPOBuyNamesTheCompanyAndTrailsTheLabel() {
        let row = facts(Item.results[2], ago: 96)
        XCTAssertEqual(row.title, "Sam bought OpenAI")
        XCTAssertEqual(row.subtitle, "Quant Kids · 4d")
        XCTAssertEqual(row.trailing, .label("Pre-IPO"))
        XCTAssertNil(row.trailing?.returnText)
        let everyone = facts(Item.results[2], .everyone, ago: 96)
        XCTAssertEqual(everyone.title, "Sam bought OpenAI $200")
        XCTAssertEqual(everyone.subtitle, "Quant Kids · 4d")
    }

    func testAPreIPOBuyWithoutANameFallsBackToTheTitle() {
        var item = Item.results[2]
        item.assetName = nil
        item.symbol = nil
        XCTAssertEqual(facts(item).title, "Sam bought \(item.title)")
    }

    func testAFailedVoteKeepsTheEnDashAndTheNoTradeLine() {
        let row = facts(Item.results[4], ago: 96)
        XCTAssertEqual(row.title, "Sell TSLA failed 2–2")
        XCTAssertEqual(row.subtitle, "Tech Titans · no trade · 4d")
        XCTAssertEqual(row.trailing, .split(yes: 2, no: 2))
        guard case .asset(let symbol, _, _, _) = row.lead else { return XCTFail("expected the logo") }
        XCTAssertEqual(symbol, "TSLAx")
    }

    func testAFailedVoteWithoutAnAssetLeadsWithTheCabal() {
        var item = Item.results[4]
        item.symbol = nil
        item.tally = nil
        let row = facts(item)
        XCTAssertEqual(row.title, "Sell failed")
        XCTAssertNil(row.trailing)
        guard case .cabal = row.lead else { return XCTFail("expected the cabal") }
    }

    func testAPassedVoteThatIsNotFilledYetSaysWhatTheTradeIsDoing() {
        var buying = Item.results[0]
        buying.fillState = nil
        let buyRow = facts(buying)
        XCTAssertEqual(buyRow.title, "Buy META passed")
        XCTAssertEqual(buyRow.subtitle, "Degens · buying · 5h")
        var selling = Item.results[1]
        selling.fillState = .pending
        XCTAssertEqual(facts(selling).subtitleParts, ["Tech Titans", "selling", "5h"])
    }

    func testAMemberJoinedHasNoTrailing() {
        let row = facts(Item.samples[4], ago: 72)
        XCTAssertEqual(row.title, "Sam joined")
        XCTAssertEqual(row.subtitle, "Weekend investors · 3d")
        XCTAssertNil(row.trailing)
    }

    func testAnOlderKindKeepsItsTitle() {
        let row = facts(Item.samples[1])
        XCTAssertEqual(row.title, Item.samples[1].title)
        XCTAssertNil(row.trailing)
    }

    func testAReplyQuotesTheCommentThenTheAge() {
        var item = Item.threads[1]
        item.action = "sell"
        item.symbol = "TSLAx"
        item.actorName = "Logan"
        let row = facts(item)
        XCTAssertEqual(row.title, "Logan on Sell TSLA")
        XCTAssertEqual(row.subtitleParts, ["“Selling into a 6% drop feels backwards.”", "5h"])
        item.body = nil
        XCTAssertEqual(facts(item).subtitleParts, ["5h"])
    }

    func testTheReplyGlyphShowsOnlyWithReplies() {
        XCTAssertEqual(facts(proposal(replies: 0)).replies, 0)
        XCTAssertFalse(facts(proposal(replies: 0)).spoken.contains("repl"))
        XCTAssertTrue(facts(proposal(replies: 1)).spoken.hasSuffix("1 reply"))
        XCTAssertTrue(facts(proposal(replies: 6)).spoken.hasSuffix("6 replies"))
    }

    func testOneSentenceReadsTitleSubLineTrailingAndReplies() {
        XCTAssertEqual(
            facts(proposal(), ago: 1).spoken,
            "AC Capital · Buy NVDA $400, 5 of 8 voted · ends in 4h, 4 yes, 1 no, 6 replies")
        XCTAssertEqual(
            facts(Item.results[1]).spoken, "Tech Titans sold AAPL, Logan called the top, 5h, $480, +6.7%, 5 replies")
        XCTAssertEqual(
            facts(Item.results[0]).spoken, "Christelle called it, Buy META, Degens, 5h, META, +1.1%, 5 replies")
        XCTAssertEqual(
            facts(Item.results[2], ago: 96).spoken, "Sam bought OpenAI, Quant Kids, 4d, Pre-IPO")
    }

    func testTheCopyLivesInResultCopy() {
        XCTAssertEqual(ResultCopy.calledTheTop("Logan"), "Logan called the top")
        XCTAssertEqual(ResultCopy.call("Logan"), "Logan’s call")
    }
}
