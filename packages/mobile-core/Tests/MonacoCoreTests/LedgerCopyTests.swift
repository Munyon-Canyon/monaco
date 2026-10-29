import Foundation
import MonacoAPI
import MonacoRanking
import XCTest

final class LedgerCopyTests: XCTestCase {
    private typealias Copy = LedgerCopy

    func testTheTitleNamesTheWeekAndCrossesAMonth() {
        XCTAssertEqual(Copy.title(start: "2026-10-12", end: "2026-10-18"), "The Ledger · Oct 12 – 18")
        XCTAssertEqual(Copy.title(start: "2026-10-26", end: "2026-11-01"), "The Ledger · Oct 26 – Nov 1")
        XCTAssertEqual(Copy.title(start: "soon", end: "2026-11-01"), "The Ledger")
        XCTAssertEqual(Copy.title(start: "2026-13-01", end: "2026-11-01"), "The Ledger")
    }

    func testThePercentIsSignedWithOneDecimal() {
        XCTAssertEqual(Copy.signedPercent(bps: 140), "+1.4%")
        XCTAssertEqual(Copy.signedPercent(bps: -80), "−0.8%")
        XCTAssertEqual(Copy.signedPercent(bps: 420), "+4.2%")
        XCTAssertEqual(Copy.signedPercent(bps: 5), "+0.1%")
        XCTAssertEqual(Copy.signedPercent(bps: 4), "0.0%")
        XCTAssertEqual(Copy.signedPercent(bps: -4), "0.0%")
        XCTAssertEqual(Copy.signedPercent(bps: 0), "0.0%")
    }

    func testTheToneFollowsTheRoundedFigure() {
        XCTAssertEqual(Copy.tone(bps: 140), .gain)
        XCTAssertEqual(Copy.tone(bps: -80), .loss)
        XCTAssertEqual(Copy.tone(bps: -4), .flat)
    }

    func testTheHeroReadsADashWhenTheReturnIsNull() {
        XCTAssertEqual(Copy.hero(bps: 140), "+1.4%")
        XCTAssertEqual(Copy.hero(bps: -80), "−0.8%")
        XCTAssertEqual(Copy.hero(bps: nil), "—")
        XCTAssertEqual(Copy.heroTone(bps: 140), .gain)
        XCTAssertEqual(Copy.heroTone(bps: nil), .flat)
    }

    func testTheHeroSubtitleCountsCabals() {
        XCTAssertEqual(Copy.heroSubtitle(cabals: 3), "your stake that week · 3 cabals")
        XCTAssertEqual(Copy.heroSubtitle(cabals: 1), "your stake that week · 1 cabal")
    }

    func testTheHeroIsSpokenForEachOutcome() {
        XCTAssertEqual(
            Copy.heroSpoken(bps: 140, cabals: 3), "Up 1.4 percent on your stake that week, across 3 cabals.")
        XCTAssertEqual(
            Copy.heroSpoken(bps: -80, cabals: 1), "Down 0.8 percent on your stake that week, across 1 cabal.")
        XCTAssertEqual(Copy.heroSpoken(bps: 0, cabals: 2), "Flat on your stake that week, across 2 cabals.")
        XCTAssertEqual(Copy.heroSpoken(bps: nil, cabals: 2), "No return for your stake that week, across 2 cabals.")
    }

    func testTheStatsReadOfAndTheSharpestFirstName() {
        XCTAssertEqual(Copy.ofTotal(3, 3), "3 of 3")
        XCTAssertEqual(Copy.ofTotal(2, 3), "2 of 3")
        XCTAssertEqual(Copy.sharpest("Christelle Dupont"), "Christelle")
        XCTAssertEqual(Copy.sharpest("Mia"), "Mia")
        XCTAssertEqual(Copy.sharpest(nil), "—")
        XCTAssertEqual(Copy.sharpest(""), "—")
    }

    func testTheCallRows() throws {
        let screen = LedgerScreen(.week42)
        let best = try XCTUnwrap(screen.bestCall)
        XCTAssertEqual(Copy.bestCallTitle(best), "Best call · Sell AMZN")
        XCTAssertEqual(Copy.bestCallSubtitle(best), "Degens · you voted Yes")
        XCTAssertEqual(Copy.signedPercent(bps: best.moveBps), "+4.2%")

        let miss = try XCTUnwrap(screen.miss)
        XCTAssertEqual(Copy.callRowTitle(miss), "Buy TSLA · Tech Titans")
        XCTAssertEqual(Copy.callRowSubtitle(miss), "you voted No · missed")
        XCTAssertEqual(Copy.signedPercent(bps: miss.moveBps), "−2.4%")
        XCTAssertEqual(Copy.callRowTitle(best), "Sell AMZN · Degens")
        XCTAssertEqual(Copy.callRowSubtitle(best), "you voted Yes · right")
    }

    func testTheReplyCaption() throws {
        let reply = try XCTUnwrap(LedgerScreen(.week42).topReply)
        XCTAssertEqual(Copy.replyCaption(reply), "🔥 3 · Buy NVDA")
        XCTAssertEqual(reply.authorName, "Christelle Dupont")
        XCTAssertEqual(reply.excerpt, "Earnings are the catalyst.")
    }

    func testWeek42BecomesTheScreenTheSheetShows() throws {
        let screen = LedgerScreen(.week42)
        XCTAssertEqual(screen.week, "2026-W42")
        XCTAssertEqual(screen.title, "The Ledger · Oct 12 – 18")
        XCTAssertEqual(Copy.hero(bps: screen.returnBps), "+1.4%")
        XCTAssertEqual(Copy.heroSubtitle(cabals: screen.cabalCount), "your stake that week · 3 cabals")
        XCTAssertEqual(Copy.ofTotal(screen.voted, screen.eligible), "3 of 3")
        XCTAssertEqual(Copy.ofTotal(screen.right, screen.resolved), "2 of 3")
        XCTAssertEqual(Copy.sharpest(screen.sharpestName), "Christelle")
        XCTAssertEqual(screen.calls.count, 3)
        XCTAssertEqual(screen.calls.first?.kind, .sell)
        XCTAssertEqual(screen.calls.dropFirst().first?.kind, .buy)
        XCTAssertEqual(screen.calls.first?.id, screen.bestCall?.proposalID)
    }

    func testNoBestCallLeavesTheHeroAndTheRowEmpty() {
        let screen = LedgerScreen(.noBestCall)
        XCTAssertNil(screen.bestCall)
        XCTAssertNil(screen.topReply)
        XCTAssertNotNil(screen.miss)
        XCTAssertEqual(Copy.hero(bps: screen.returnBps), "—")
        XCTAssertEqual(Copy.ofTotal(screen.voted, screen.eligible), "1 of 3")
        XCTAssertEqual(Copy.ofTotal(screen.right, screen.resolved), "0 of 1")
        XCTAssertEqual(Copy.noBestCallTitle, "No call landed this week")
        XCTAssertEqual(Copy.noBestCallSubtitle, "Every call is in the Full Ledger")
    }

    func testALossReadsMinusInTheLossTone() {
        let screen = LedgerScreen(.loss)
        XCTAssertEqual(Copy.hero(bps: screen.returnBps), "−0.8%")
        XCTAssertEqual(Copy.heroTone(bps: screen.returnBps), .loss)
    }

    func testOnlyLedgerNotFoundIsNotReady() {
        let missing = Components.Schemas.Problem(
            _type: .about_colon_blank, title: "Error", status: 404, code: .ledgerNotFound, message: "None.",
            traceId: "4bf92f3577b34da6a3ce929d0e0e4736", retryable: false)
        let other = Components.Schemas.Problem(
            _type: .about_colon_blank, title: "Error", status: 503, code: .upstreamUnavailable, message: "No.",
            traceId: "4bf92f3577b34da6a3ce929d0e0e4736", retryable: false)
        XCTAssertTrue(Copy.isNotReady(.problem(ProblemError(missing))))
        XCTAssertFalse(Copy.isNotReady(.problem(ProblemError(other))))
        XCTAssertFalse(Copy.isNotReady(.transport(URLError(.notConnectedToInternet))))
        XCTAssertEqual(Copy.notReady, "This week's Ledger isn't ready yet.")
        XCTAssertEqual(Copy.fullLedgerSubtitle, "Every call, the miss and the top reply")
    }
}
