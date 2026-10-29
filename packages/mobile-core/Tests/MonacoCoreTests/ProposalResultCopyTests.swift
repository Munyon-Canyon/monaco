import MonacoAPI
import XCTest

@testable import MonacoCore

@MainActor
final class ProposalResultCopyTests: XCTestCase {
    private let fixedNow = Date(timeIntervalSince1970: 1_792_662_780)
    private let utc: Calendar = {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = TimeZone(secondsFromGMT: 0) ?? .current
        return calendar
    }()

    private func swap(
        _ status: Components.Schemas.ProposalDetail.SwapPayload.StatusPayload = .confirmed, filledAt: Date? = nil,
        feeMicros: Int64? = 400_000, tokenAmount: Int64? = 182_000_000, price: Int64? = 219_400_000,
        message: String? = nil
    ) -> ProposalSwap {
        ProposalSwap(
            .init(
                swapId: "swap-1", status: status, failureCode: nil, failureMessage: message, txSignature: nil,
                retryable: false, usdcMicros: nil, tokenAmount: tokenAmount, tokenDecimals: 8,
                priceMicros: price, feeMicros: feeMicros, filledAt: filledAt))
    }

    private func filledSwap() -> ProposalSwap {
        swap(filledAt: Date(timeIntervalSince1970: 1_792_662_780 - 720))
    }

    private let tally = ProposalTally(yes: 5, no: 1, voters: 8, needed: 5)

    func testHeadlines() {
        let cases: [(ProposalStatus, ProposalTally, String)] = [
            (.passed, tally, "Passed 5–1"), (.executed, tally, "Passed 5–1"),
            (.executionBlocked, tally, "Passed 5–1"),
            (.failed, ProposalTally(yes: 2, no: 2, voters: 5, needed: 3), "Failed 2–2"),
            (.expired, ProposalTally(yes: 3, no: 1, voters: 5, needed: 4), "Expired 3–1"),
            (.expired, ProposalTally(yes: 0, no: 0, voters: 5, needed: 4), "Expired"),
            (.withdrawn, tally, "Withdrawn"), (.voided, tally, "Voided by Monaco"),
            (.open, tally, "Voting open"),
        ]
        for (status, tally, expected) in cases {
            XCTAssertEqual(ProposalResultCopy.headline(status: status, tally: tally), expected)
        }
    }

    func testSeals() {
        XCTAssertEqual(ProposalResultCopy.seal(status: .passed), .passed)
        XCTAssertEqual(ProposalResultCopy.seal(status: .executed), .passed)
        XCTAssertEqual(ProposalResultCopy.seal(status: .failed), .failed)
        XCTAssertEqual(ProposalResultCopy.seal(status: .expired), .neutral)
    }

    func testTheTallyLine() {
        XCTAssertEqual(ProposalResultCopy.tallyLine(tally: tally), "✓ 5 · 6 of 8 voted · 1 ✕")
    }

    func testUnits() {
        XCTAssertEqual(ProposalResultCopy.units(atomics: 182_000_000, decimals: 8), "1.82")
        XCTAssertEqual(ProposalResultCopy.units(atomics: 220_000_000, decimals: 8), "2.2")
        XCTAssertNil(ProposalResultCopy.units(atomics: -1, decimals: 8))
        XCTAssertNil(ProposalResultCopy.units(atomics: 1, decimals: 19))
        XCTAssertEqual(ProposalResultCopy.units(atomics: 5, decimals: 0), "5")
        XCTAssertEqual(ProposalResultCopy.units(atomics: 105_000_000, decimals: 8), "1.05")
        XCTAssertEqual(ProposalResultCopy.units(atomics: 181_600_000, decimals: 8), "1.82")
    }

    func testFilledNamesOnlyAConfirmedFill() {
        let fill = filledSwap()
        XCTAssertEqual(ProposalResultCopy.filled(fill, unit: .shares), "Bought 1.82 sh at $219.40")
        XCTAssertEqual(ProposalResultCopy.filled(fill, unit: .ticker("NVDA")), "Bought 1.82 NVDA at $219.40")
        XCTAssertEqual(ProposalResultCopy.filled(fill, unit: .shares, isSell: true), "Sold 1.82 sh at $219.40")
        let at = Date(timeIntervalSince1970: 1_792_662_000)
        XCTAssertNil(ProposalResultCopy.filled(swap(.created, filledAt: at), unit: .shares))
        XCTAssertNil(ProposalResultCopy.filled(swap(.submitted, filledAt: at), unit: .shares))
        XCTAssertNil(ProposalResultCopy.filled(swap(.failed, filledAt: at), unit: .shares))
        XCTAssertNil(ProposalResultCopy.filled(swap(.confirmed), unit: .shares))
        XCTAssertNil(ProposalResultCopy.filled(swap(filledAt: at, price: nil), unit: .shares))
        XCTAssertNil(ProposalResultCopy.filled(swap(filledAt: at, tokenAmount: nil), unit: .shares))
    }

    func testCaptions() {
        func caption(
            _ status: ProposalStatus, _ swap: ProposalSwap? = nil, sell: Bool = false, units: String? = "1.82 sh",
            message: String? = nil
        ) -> String? {
            ProposalResultCopy.caption(
                status: status, tally: ProposalTally(yes: 2, no: 2, voters: 5, needed: 3), swap: swap, isSell: sell,
                pendingUnits: units, statusMessage: message, calendar: utc)
        }
        XCTAssertEqual(caption(.executed, filledSwap()), "filled at $219.40 · Oct 22")
        XCTAssertEqual(caption(.passed), "buying 1.82 sh · in progress")
        XCTAssertEqual(caption(.passed, sell: true), "selling 1.82 sh · in progress")
        XCTAssertEqual(caption(.passed, units: nil), "in progress")
        XCTAssertEqual(caption(.failed), "needed 3 · no trade")
        XCTAssertEqual(caption(.expired), "closed before 3 yes · no trade")
        XCTAssertEqual(caption(.executionBlocked, message: "Paused"), "Paused")
        XCTAssertNil(caption(.withdrawn))
        XCTAssertNil(caption(.voided))
        XCTAssertNil(caption(.open))
    }

    func testTheTradeRow() async throws {
        let row = try XCTUnwrap(
            ProposalResultCopy.tradeRow(swap: filledSwap(), isSell: false, sinceBps: 30, calendar: utc))
        XCTAssertEqual(row.title, "Bought 1.82 sh at $219.40")
        XCTAssertEqual(row.subtitle, "Filled 9:41 AM · fee $0.40")
        XCTAssertEqual(row.trailing, "+0.3%")
        XCTAssertEqual(row.tone, .gain)
        let free = swap(filledAt: fixedNow, feeMicros: nil)
        let noFee = try XCTUnwrap(ProposalResultCopy.tradeRow(swap: free, isSell: false, sinceBps: -250, calendar: utc))
        XCTAssertEqual(noFee.subtitle, "Filled 9:53 AM")
        XCTAssertEqual(noFee.trailing, "−2.5%")
        XCTAssertEqual(noFee.tone, .loss)
        let flat = try XCTUnwrap(ProposalResultCopy.tradeRow(swap: free, isSell: false, sinceBps: 0, calendar: utc))
        XCTAssertEqual(flat.trailing, "0%")
        XCTAssertEqual(flat.tone, .plain)
        let unknown = try XCTUnwrap(
            ProposalResultCopy.tradeRow(swap: free, isSell: false, sinceBps: nil, calendar: utc))
        XCTAssertNil(unknown.trailing)
        XCTAssertEqual(unknown.tone, .plain)
        XCTAssertNil(ProposalResultCopy.tradeRow(swap: swap(.submitted), isSell: false, sinceBps: 30))
    }

    func testThePendingTradeRowNeverClaimsAFill() {
        let pending = ProposalResultCopy.pendingTradeRow(
            swap: swap(.submitted), isSell: false, units: "1.82 sh", statusMessage: nil)
        XCTAssertEqual(pending, .init(title: "Buying 1.82 sh", subtitle: "In progress"))
        let sell = ProposalResultCopy.pendingTradeRow(swap: nil, isSell: true, units: nil, statusMessage: nil)
        XCTAssertEqual(sell, .init(title: "Selling", subtitle: "In progress"))
        let failed = ProposalResultCopy.pendingTradeRow(
            swap: swap(.failed, message: "Price moved."), isSell: false, units: "1.82 sh", statusMessage: nil)
        XCTAssertEqual(failed.subtitle, "Price moved.")
        let unnamed = ProposalResultCopy.pendingTradeRow(
            swap: swap(.failed), isSell: false, units: nil, statusMessage: nil)
        XCTAssertEqual(unnamed.subtitle, "The trade did not go through.")
        let blocked = ProposalResultCopy.pendingTradeRow(
            swap: nil, isSell: false, units: "1.82 sh", statusMessage: "Trading is paused.")
        XCTAssertEqual(blocked.subtitle, "Trading is paused.")
    }

    func testMyRow() {
        func row(_ vote: ProposalMyVote?, _ choice: ProposalBallot?, _ status: ProposalStatus)
            -> ProposalResultCopy.Row?
        {
            ProposalResultCopy.myRow(myVote: vote, choice: choice, status: status)
        }
        let decided = ProposalMyVote(order: 6, of: 8, yesOrder: 5, noOrder: 3, decided: true)
        let plain = ProposalMyVote(order: 4, of: 6, yesOrder: 4, noOrder: 0, decided: false)
        XCTAssertEqual(row(decided, .yes, .executed)?.title, "You voted Yes")
        XCTAssertEqual(row(decided, .yes, .passed)?.subtitle, "The 5th yes · it passed")
        XCTAssertEqual(row(decided, .no, .failed)?.title, "You voted No")
        XCTAssertEqual(row(decided, .no, .failed)?.subtitle, "The 3rd no · it failed")
        XCTAssertEqual(row(plain, .yes, .executed)?.subtitle, "4th of 6")
        XCTAssertEqual(row(decided, .no, .expired)?.subtitle, "6th of 8")
        XCTAssertEqual(row(nil, .yes, .passed), .init(title: "You voted Yes", subtitle: nil))
        XCTAssertNil(row(plain, nil, .passed))
    }

    func testTheNotVotedRow() {
        XCTAssertEqual(ProposalResultCopy.notVotedRow(names: ["Priya"], status: .failed), "Priya did not vote")
        XCTAssertEqual(
            ProposalResultCopy.notVotedRow(names: ["Jordan", "Priya"], status: .expired),
            "Jordan and Priya did not vote")
        XCTAssertEqual(
            ProposalResultCopy.notVotedRow(names: ["Jordan", "Priya", "Kai", "Sam"], status: .failed),
            "Jordan, Priya and 2 more did not vote")
        XCTAssertNil(ProposalResultCopy.notVotedRow(names: [], status: .failed))
        XCTAssertNil(ProposalResultCopy.notVotedRow(names: ["Priya"], status: .passed))
    }

    func testTheAction() {
        XCTAssertEqual(ProposalResultCopy.action(status: .passed, canPropose: false), .share)
        XCTAssertEqual(ProposalResultCopy.action(status: .executed, canPropose: true), .share)
        XCTAssertNil(ProposalResultCopy.action(status: .executionBlocked, canPropose: true))
        XCTAssertEqual(ProposalResultCopy.action(status: .failed, canPropose: true), .proposeAgain)
        XCTAssertEqual(ProposalResultCopy.action(status: .expired, canPropose: true), .proposeAgain)
        XCTAssertNil(ProposalResultCopy.action(status: .failed, canPropose: false))
        XCTAssertNil(ProposalResultCopy.action(status: .withdrawn, canPropose: true))
    }
}
