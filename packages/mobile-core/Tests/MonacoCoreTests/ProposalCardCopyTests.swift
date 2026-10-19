import XCTest

@testable import MonacoCore

final class ProposalCardCopyTests: XCTestCase {
    func testClosesInHoursAndMinutes() {
        let now = Date(timeIntervalSince1970: 0)
        XCTAssertEqual(ProposalCardCopy.closes(at: now.addingTimeInterval(23 * 3600), now: now), "Closes in 23h")
        XCTAssertEqual(ProposalCardCopy.closes(at: now.addingTimeInterval(59 * 60), now: now), "Closes in 59m")
        XCTAssertEqual(ProposalCardCopy.closes(at: now.addingTimeInterval(60), now: now), "Closes in 1m")
        XCTAssertEqual(ProposalCardCopy.closes(at: now.addingTimeInterval(-60), now: now), "Voting closed")
    }

    func testClosesInDaysFromTwoDaysOut() {
        let now = Date(timeIntervalSince1970: 0)
        XCTAssertEqual(ProposalCardCopy.closes(at: now.addingTimeInterval(167 * 3600), now: now), "Closes in 6d")
        XCTAssertEqual(ProposalCardCopy.closes(at: now.addingTimeInterval(47 * 3600), now: now), "Closes in 47h")
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
            ProposalCardCopy.pausedCaption(isSell: false),
            "Trading is paused. If this passes, it won't buy until trading resumes.")
        XCTAssertEqual(
            ProposalCardCopy.pausedCaption(isSell: true),
            "Trading is paused. If this passes, it won't sell until trading resumes.")
    }

    func testAge() {
        let now = Date(timeIntervalSince1970: 100_000)
        XCTAssertEqual(ProposalCardCopy.age(since: now.addingTimeInterval(-3 * 3600), now: now), "3h")
        XCTAssertEqual(ProposalCardCopy.age(since: now.addingTimeInterval(-50 * 3600), now: now), "2d")
        XCTAssertEqual(ProposalCardCopy.age(since: now.addingTimeInterval(60), now: now), "now")
        XCTAssertEqual(ProposalCardCopy.age(since: now.addingTimeInterval(-30), now: now), "now")
        XCTAssertEqual(ProposalCardCopy.age(since: now.addingTimeInterval(-5 * 60), now: now), "5m")
    }

    func testAgoReadsAsAPastTimeAndJustNowUnderAMinute() {
        let now = Date(timeIntervalSince1970: 100_000)
        XCTAssertEqual(ProposalCardCopy.ago(since: now.addingTimeInterval(-2 * 3600), now: now), "2h ago")
        XCTAssertEqual(ProposalCardCopy.ago(since: now.addingTimeInterval(-5 * 60), now: now), "5m ago")
        XCTAssertEqual(ProposalCardCopy.ago(since: now.addingTimeInterval(-30), now: now), "Just now")
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

    func testExpectedForExactlyOneIsSingular() {
        XCTAssertEqual(
            ProposalCardCopy.expected(
                isSell: false, quoteOut: 100_000_000, usdcMicros: 100_000_000, decimals: 8, kind: .stock),
            "about 1 share at $100.00")
        XCTAssertEqual(
            ProposalCardCopy.expected(
                isSell: false, quoteOut: 100_000_000, usdcMicros: 100_000_000, decimals: 8, kind: .preIpo),
            "about 1 token at $100.00")
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

    private let tally = ProposalTally(yes: 3, no: 1, voters: 8, needed: 5)

    func testButtonForEveryState() {
        let table: [(ProposalState, String, Bool)] = [
            (.open, "Review", true), (.voted(.yes), "Review vote", false), (.voted(.no), "Review vote", false),
            (.viewOnly, "Review", false), (.approved, "View status", false), (.executed, "View trade", false),
            (.expired, "View decision", false), (.blocked, "View status", false),
            (.rejected, "View decision", false), (.withdrawn, "View decision", false),
            (.voided, "View decision", false),
        ]
        for (state, title, isPrimary) in table {
            XCTAssertEqual(ProposalCardCopy.button(state), .init(title: title, isPrimary: isPrimary), "\(state)")
        }
    }

    func testOpenNoteNamesTheYesVotesNeededAndWhenItCloses() {
        let now = Date(timeIntervalSince1970: 0)
        let expiry = now.addingTimeInterval(8 * 3600)
        for state in [ProposalState.open, .voted(.yes), .voted(.no), .viewOnly] {
            XCTAssertEqual(
                ProposalCardCopy.note(state, needed: 5, expiresAt: expiry, statusMessage: nil, now: now),
                "5 yes votes needed · Closes in 8h", "\(state)")
        }
    }

    func testNeededIsSingularForOne() {
        XCTAssertEqual(ProposalCardCopy.neededLabel(1), "1 yes vote needed")
        XCTAssertEqual(ProposalCardCopy.neededLabel(2), "2 yes votes needed")
        let now = Date(timeIntervalSince1970: 0)
        XCTAssertEqual(
            ProposalCardCopy.note(
                .open, needed: 1, expiresAt: now.addingTimeInterval(59 * 60), statusMessage: nil, now: now),
            "1 yes vote needed · Closes in 59m")
    }

    func testClosedNotes() {
        let now = Date(timeIntervalSince1970: 0)
        func note(_ state: ProposalState, _ message: String? = nil) -> String {
            ProposalCardCopy.note(state, needed: 2, expiresAt: now, statusMessage: message, now: now)
        }
        XCTAssertEqual(note(.approved), "The cabal passed this proposal.")
        XCTAssertEqual(note(.executed), "The trade is complete.")
        XCTAssertEqual(note(.expired), "Voting ended without enough yes votes.")
        XCTAssertEqual(note(.blocked), "Execution was blocked. Review the status.")
        XCTAssertEqual(note(.rejected), "The proposal did not pass.")
        XCTAssertEqual(note(.withdrawn), "The proposer withdrew this proposal.")
        XCTAssertEqual(note(.voided), "Monaco voided this proposal.")
        XCTAssertEqual(note(.voided, ""), "Monaco voided this proposal.")
        XCTAssertEqual(note(.voided, "The cabal was closed."), "The cabal was closed.")
    }

    func testFooterForEveryState() {
        let table: [(ProposalState, ProposalCardCopy.FooterIcon, String, ProposalCardCopy.FooterTone)] = [
            (.open, .chat, "3 yes · 1 no · 4 not voted", .muted),
            (.voted(.yes), .chat, "3 yes · 1 no · 4 not voted", .muted),
            (.viewOnly, .chat, "3 yes · 1 no · 4 not voted", .muted),
            (.approved, .clock, "Approved by the cabal · Not completed", .warning),
            (.executed, .done, "Trade confirmed · View the original decision", .gain),
            (.expired, .clock, "Voting closed · No trade placed", .muted),
            (.blocked, .warning, "Decision retained · No completed trade", .loss),
            (.rejected, .clock, "Voting closed · No trade placed", .muted),
            (.withdrawn, .clock, "Voting closed · No trade placed", .muted),
            (.voided, .clock, "Voting closed · No trade placed", .muted),
        ]
        for (state, icon, text, tone) in table {
            let footer = ProposalCardCopy.footer(state, tally: tally)
            XCTAssertEqual(footer.icon, icon, "\(state)")
            XCTAssertEqual(footer.text, text, "\(state)")
            XCTAssertEqual(footer.tone, tone, "\(state)")
        }
    }

    func testTallyLineCountsTheVotersWhoHaveNotVoted() {
        XCTAssertEqual(
            ProposalCardCopy.tallyLine(ProposalTally(yes: 0, no: 0, voters: 3, needed: 2)), "0 yes · 0 no · 3 not voted"
        )
        XCTAssertEqual(
            ProposalCardCopy.tallyLine(ProposalTally(yes: 5, no: 1, voters: 8, needed: 5)), "5 yes · 1 no · 2 not voted"
        )
        XCTAssertEqual(
            ProposalCardCopy.tallyLine(ProposalTally(yes: 2, no: 2, voters: 3, needed: 2)), "2 yes · 2 no · 0 not voted"
        )
    }

    func testRowSubtitleForEveryState() {
        func subtitle(_ state: ProposalState, _ message: String? = nil) -> String {
            ProposalCardCopy.rowSubtitle(state, proposer: "Mia", statusMessage: message)
        }
        for state in [ProposalState.open, .voted(.yes), .viewOnly] {
            XCTAssertEqual(subtitle(state), "Mia · Voting open")
        }
        XCTAssertEqual(subtitle(.approved), "Approved · execution pending")
        XCTAssertEqual(subtitle(.executed), "Trade confirmed")
        XCTAssertEqual(subtitle(.expired), "Voting ended without enough yes votes")
        XCTAssertEqual(subtitle(.blocked), "The trade did not go through")
        XCTAssertEqual(subtitle(.rejected), "The proposal did not pass")
        XCTAssertEqual(subtitle(.withdrawn), "The proposer withdrew this proposal")
        XCTAssertEqual(subtitle(.voided), "Monaco voided this proposal")
        XCTAssertEqual(subtitle(.voided, "Closed by Monaco"), "Closed by Monaco")
    }

    func testStatusNoticeForEveryClosedState() {
        func notice(_ state: ProposalState, failure: String? = nil, message: String? = nil) -> ProposalCardCopy.Notice?
        {
            ProposalCardCopy.statusNotice(state, failureMessage: failure, statusMessage: message)
        }
        for state in [ProposalState.open, .voted(.no), .viewOnly] { XCTAssertNil(notice(state)) }
        XCTAssertEqual(
            notice(.approved), .init(tone: .warn, message: "The vote passed. Execution is pending."))
        XCTAssertEqual(notice(.executed), .init(tone: .info, message: "The trade is confirmed."))
        XCTAssertEqual(
            notice(.expired),
            .init(tone: .info, message: "Voting closed without enough yes votes. No trade was placed."))
        XCTAssertEqual(notice(.rejected), .init(tone: .info, message: "The proposal did not pass."))
        XCTAssertEqual(
            notice(.withdrawn),
            .init(tone: .info, message: "Withdrawn."))
        XCTAssertEqual(
            notice(.voided, message: "Closed by Monaco"),
            .init(tone: .info, message: "Voided by Monaco. Closed by Monaco."))
    }

    func testBlockedNoticeAddsTheFailureAsASecondSentence() {
        func notice(failure: String?, message: String?) -> String? {
            ProposalCardCopy.statusNotice(.blocked, failureMessage: failure, statusMessage: message)?.message
        }
        XCTAssertEqual(notice(failure: nil, message: nil), "The trade did not go through.")
        XCTAssertEqual(
            notice(failure: "Slippage was too high", message: "Ignored"),
            "The trade did not go through. Slippage was too high.")
        XCTAssertEqual(
            notice(failure: nil, message: "The pot was empty."), "The trade did not go through. The pot was empty.")
        XCTAssertEqual(
            notice(failure: "The trade did not go through.", message: nil), "The trade did not go through.")
        XCTAssertEqual(notice(failure: "", message: ""), "The trade did not go through.")
        XCTAssertEqual(
            ProposalCardCopy.statusNotice(.blocked, failureMessage: nil, statusMessage: nil)?.tone, .error)
    }
}
