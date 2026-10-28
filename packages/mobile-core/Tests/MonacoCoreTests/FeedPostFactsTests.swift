import MonacoAPI
import MonacoCore
import XCTest

final class FeedPostFactsTests: XCTestCase {
    private typealias Item = Components.Schemas.FeedItem

    private let now = Date(timeIntervalSinceReferenceDate: 800_000_000)

    func testAnOpenProposalReadsAsTheFrameDraws() {
        let facts = FeedPostFacts(proposal(), now: now)
        XCTAssertEqual(facts.lead, .person(name: "Maya", photoURL: nil, userID: "00000000-0000-7000-8000-0000000fb001"))
        XCTAssertEqual(facts.name, "Maya")
        XCTAssertEqual(facts.verb, "proposed")
        XCTAssertEqual(facts.age, "2h")
        XCTAssertEqual(facts.object?.symbol, "AAPLx")
        XCTAssertEqual(facts.object?.headline, "Buy AAPL $500")
        XCTAssertEqual(facts.object?.place, "Weekend investors")
        XCTAssertNil(facts.object?.trailing)
        XCTAssertEqual(facts.object?.showsLogo, true)
        XCTAssertEqual(facts.accentCabalID, Item.sampleCabalID)
        XCTAssertEqual(
            facts.attachment,
            .tally(.init(yes: 4, no: 1, voters: 8, needed: 5, statusLine: "5 of 8 voted · ends in 4h")))
    }

    func testTheSpokenLabelReadsOneSentence() {
        XCTAssertEqual(
            FeedPostFacts(proposal(), now: now).spoken,
            "Maya proposed Buy AAPL, 500 dollars, in Weekend investors, 2 hours ago. 4 yes, 1 no. "
                + "5 of 8 voted, ends in 4 hours.")
    }

    func testAHeadlineDropsTheCentsOnlyWhenThereAreNone() {
        XCTAssertEqual(headline(micros: 400_000_000), "Buy AAPL $400")
        XCTAssertEqual(headline(micros: 400_500_000), "Buy AAPL $400.50")
        XCTAssertEqual(headline(micros: 1_200_000_000), "Buy AAPL $1,200")
    }

    func testASellBySharesNamesTheShares() {
        var item = proposal()
        item.action = "sell"
        item.symbol = "TSLAx"
        item.usdcMicros = nil
        item.tokenAmount = 200_000_000
        item.tokenDecimals = 8
        let facts = FeedPostFacts(item, now: now)
        XCTAssertEqual(facts.object?.headline, "Sell TSLA 2 shares")
        XCTAssertTrue(facts.spoken.hasPrefix("Maya proposed Sell TSLA, 2 shares, in Weekend investors"))
    }

    func testAPreIPOProposalCarriesItsTag() {
        var item = proposal()
        item.symbol = "OPENAI"
        item.assetKind = .preIpo
        let facts = FeedPostFacts(item, now: now)
        XCTAssertEqual(facts.object?.headline, "Buy OPENAI $500")
        XCTAssertEqual(facts.object?.trailing, .text("Pre-IPO"))
    }

    func testAFigureTravelsToTheTrailingEnd() {
        var item = proposal()
        item.changeBps = 110
        XCTAssertEqual(FeedPostFacts(item, now: now).object?.trailing, .figure(110))
    }

    func testAClosedProposalNamesItsOutcomeAndTheVoteCount() {
        var passed = proposal()
        passed.status = "passed"
        XCTAssertEqual(statusLine(passed), "Passed 4–1 · 5 of 8 voted")
        var failed = proposal()
        failed.status = "failed"
        failed.tally = .init(yes: 2, no: 2, voters: 5, needed: 3)
        XCTAssertEqual(statusLine(failed), "Failed 2–2 · 4 of 5 voted")
        var expired = failed
        expired.status = "expired"
        XCTAssertEqual(statusLine(expired), "Failed 2–2 · 4 of 5 voted")
        var withdrawn = proposal()
        withdrawn.status = "withdrawn"
        XCTAssertEqual(statusLine(withdrawn), "Withdrawn · 5 of 8 voted")
    }

    func testAnOpenProposalCountsDownInTheLargestWholeUnit() {
        XCTAssertEqual(statusLine(proposal(endsIn: 30 * 60)), "5 of 8 voted · ends in 30m")
        XCTAssertEqual(statusLine(proposal(endsIn: 3 * 3600 + 1)), "5 of 8 voted · ends in 4h")
        XCTAssertEqual(statusLine(proposal(endsIn: 30 * 3600)), "5 of 8 voted · ends in 2d")
        XCTAssertEqual(statusLine(proposal(endsIn: -5)), "5 of 8 voted · ending now")
    }

    func testAProposalWithNoTallyHasNoAttachment() {
        var item = proposal()
        item.tally = nil
        XCTAssertEqual(FeedPostFacts(item, now: now).attachment, .none)
    }

    func testAJoinNamesTheCabalOnAnObjectLineWithNoLogo() {
        let facts = FeedPostFacts(item(at: 4, age: 3 * 86_400), now: now)
        XCTAssertEqual(facts.name, "Sam")
        XCTAssertEqual(facts.verb, "joined")
        XCTAssertEqual(facts.age, "3d")
        XCTAssertEqual(facts.object?.headline, "Weekend investors")
        XCTAssertEqual(facts.object?.showsLogo, false)
        XCTAssertNil(facts.object?.place)
        XCTAssertEqual(facts.attachment, .none)
        XCTAssertEqual(facts.spoken, "Sam joined Weekend investors, 3 days ago.")
    }

    func testACabalStartIsAJoinWithItsOwnVerb() {
        let facts = FeedPostFacts(item(at: 3, age: 5 * 3600), now: now)
        XCTAssertEqual(facts.name, "Jordan")
        XCTAssertEqual(facts.verb, "started")
        XCTAssertEqual(facts.object?.headline, "Weekend investors")
        XCTAssertEqual(facts.age, "5h")
    }

    func testAReplyQuotesTheCommentUnderTheProposalItAnswers() {
        var item = item(at: 0, age: 3 * 86_400)
        item.kind = "reply"
        item.action = "sell"
        item.symbol = "TSLAx"
        item.body = "Fine, half next week then."
        item.tally = nil
        let facts = FeedPostFacts(item, now: now)
        XCTAssertEqual(facts.name, "Maya")
        XCTAssertEqual(facts.verb, "on Sell TSLA")
        XCTAssertNil(facts.object)
        XCTAssertEqual(facts.attachment, .quote("“Fine, half next week then.”"))
        XCTAssertEqual(facts.spoken, "Maya on Sell TSLA, 3 days ago. “Fine, half next week then.”")
    }

    func testAReplyWithNoProposalDetailJustReplied() {
        var item = item(at: 0, age: 60)
        item.kind = "reply"
        item.symbol = nil
        item.body = nil
        let facts = FeedPostFacts(item, now: now)
        XCTAssertEqual(facts.verb, "replied")
        XCTAssertEqual(facts.attachment, .none)
    }

    func testAnOldTradeKeepsTheServerTitleAsItsHeadline() {
        let facts = FeedPostFacts(item(at: 1, age: 3600), now: now)
        XCTAssertEqual(facts.verb, "")
        XCTAssertEqual(facts.object?.headline, "Weekend investors bought $250 of NVDAx")
        XCTAssertEqual(facts.object?.showsLogo, false)
        XCTAssertEqual(facts.attachment, .none)
    }

    func testAResultIsHeadedByTheCabalOrTheMemberItsShapeNames() {
        let cabal = FeedPostFacts(Item.results[0], now: now)
        XCTAssertEqual(cabal.lead, .cabal(name: "Degens", pictureURL: nil, cabalID: Item.results[0].cabalId))
        let member = FeedPostFacts(Item.results[1], now: now)
        XCTAssertEqual(member.name, "Logan")
    }

    func testAPostWithNoActorIsHeadedByItsCabal() {
        var item = item(at: 1, age: 60)
        item.actorId = nil
        item.actorName = nil
        XCTAssertEqual(
            FeedPostFacts(item, now: now).lead,
            .cabal(name: "Weekend investors", pictureURL: nil, cabalID: item.cabalId))
    }

    func testAgeIsCompactThenADate() {
        XCTAssertEqual(age(30), "now")
        XCTAssertEqual(age(5 * 60), "5m")
        XCTAssertEqual(age(2 * 3600), "2h")
        XCTAssertEqual(age(3 * 86_400), "3d")
        let old = now.addingTimeInterval(-10 * 86_400)
        XCTAssertEqual(age(10 * 86_400), RelativeTimeFormatter.label(date: old, now: now))
    }

    private func age(_ seconds: TimeInterval) -> String {
        FeedPostFacts(item(at: 4, age: seconds), now: now).age
    }

    private func statusLine(_ item: Item) -> String? {
        guard case .tally(let tally) = FeedPostFacts(item, now: now).attachment else { return nil }
        return tally.statusLine
    }

    private func headline(micros: Int64) -> String? {
        var item = proposal()
        item.usdcMicros = micros
        return FeedPostFacts(item, now: now).object?.headline
    }

    private func proposal(endsIn: TimeInterval = 4 * 3600) -> Item {
        var item = item(at: 0, age: 2 * 3600)
        item.action = "buy"
        item.usdcMicros = 500_000_000
        item.expiresAt = now.addingTimeInterval(endsIn)
        return item
    }

    private func item(at index: Int, age: TimeInterval) -> Item {
        var item = Item.samples[index]
        item.createdAt = now.addingTimeInterval(-age)
        item.updatedAt = item.createdAt
        return item
    }
}
