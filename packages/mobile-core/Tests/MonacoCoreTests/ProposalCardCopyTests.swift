import MonacoCore
import XCTest

final class ProposalCardCopyTests: XCTestCase {
    func testClosesInHoursAndMinutes() {
        let now = Date(timeIntervalSince1970: 0)
        XCTAssertEqual(ProposalCardCopy.closes(at: now.addingTimeInterval(23 * 3600), now: now), "Closes in 23h")
        XCTAssertEqual(ProposalCardCopy.closes(at: now.addingTimeInterval(59 * 60), now: now), "Closes in 59m")
        XCTAssertEqual(ProposalCardCopy.closes(at: now.addingTimeInterval(60), now: now), "Closes in 1m")
        XCTAssertEqual(ProposalCardCopy.closes(at: now.addingTimeInterval(-60), now: now), "Closes in 1m")
    }

    func testVoteCopy() {
        XCTAssertEqual(ProposalCardCopy.tracker(voted: 1, voters: 3, needed: 2), "1 of 3 voted · 2 yes to pass")
        XCTAssertEqual(ProposalCardCopy.voter("Priya", choice: "yes"), "Priya voted yes")
        XCTAssertEqual(ProposalCardCopy.voter("Priya", choice: "no"), "Priya voted no")
        XCTAssertEqual(ProposalCardCopy.voter("Priya", choice: nil), "Priya hasn't voted")
        XCTAssertEqual(ProposalCardCopy.viewerVote("yes"), "✓ You voted yes")
        XCTAssertEqual(ProposalCardCopy.viewerVote("no"), "✓ You voted no")
        XCTAssertNil(ProposalCardCopy.viewerVote(nil))
    }

    func testSellAmountUsesSharesOrTokens() {
        XCTAssertEqual(ProposalCardCopy.sellAmount(atomics: "60170000", decimals: 8, kind: .stock), "0.6017 shares")
        XCTAssertEqual(ProposalCardCopy.sellAmount(atomics: "60170000", decimals: 8, kind: .preIpo), "0.6017 tokens")
    }

    func testCaptionCopyIsTheScreenMapSentence() async throws {
        XCTAssertEqual(
            ProposalCardCopy.pausedCaption, "Trading is paused. If this passes, it won't buy until trading resumes.")
    }

    func testAge() {
        let now = Date(timeIntervalSince1970: 100_000)
        XCTAssertEqual(ProposalCardCopy.age(since: now.addingTimeInterval(-3 * 3600), now: now), "3h")
        XCTAssertEqual(ProposalCardCopy.age(since: now.addingTimeInterval(-50 * 3600), now: now), "2d")
        XCTAssertEqual(ProposalCardCopy.age(since: now.addingTimeInterval(60), now: now), "0m")
    }

    func testExpectedForABuyNamesSharesAndPrice() {
        XCTAssertEqual(
            ProposalCardCopy.expected(
                isSell: false, quoteOut: 73_000_000, usdcMicros: 249_366_100, decimals: 8, kind: .stock),
            "about 0.73 shares at $341.60")
        XCTAssertEqual(
            ProposalCardCopy.expected(
                isSell: false, quoteOut: 73_000_000, usdcMicros: 249_366_100, decimals: 8, kind: .preIpo),
            "about 0.73 tokens at $341.60")
        XCTAssertEqual(
            ProposalCardCopy.expected(
                isSell: false, quoteOut: 123_456_789, usdcMicros: 100_000_000, decimals: 8, kind: .stock),
            "about 1.2346 shares at $81.00")
    }

    func testExpectedForASellIsTheDollarProceeds() {
        XCTAssertEqual(
            ProposalCardCopy.expected(isSell: true, quoteOut: 139_000_000, usdcMicros: nil, decimals: 8, kind: .stock),
            "about $139.00")
    }

    func testExpectedIsNilWithoutAQuote() {
        XCTAssertNil(
            ProposalCardCopy.expected(isSell: false, quoteOut: 0, usdcMicros: 1, decimals: 8, kind: .stock))
        XCTAssertNil(
            ProposalCardCopy.expected(isSell: false, quoteOut: 1, usdcMicros: nil, decimals: 8, kind: .stock))
        XCTAssertNil(
            ProposalCardCopy.expected(isSell: true, quoteOut: 0, usdcMicros: nil, decimals: 8, kind: .stock))
    }
}
