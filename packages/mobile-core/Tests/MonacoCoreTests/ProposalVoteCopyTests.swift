import XCTest

@testable import MonacoCore

final class ProposalVoteCopyTests: XCTestCase {
    private let now = Date(timeIntervalSince1970: 1_800_000_000)
    private let tally = ProposalTally(yes: 4, no: 1, voters: 8, needed: 5)

    private func outcome(
        _ choice: ProposalBallot = .yes, status: ProposalStatus = .open, yes: Int = 5, no: Int = 1,
        undo: Bool = true, order: Int = 6, changed: Bool = false, myVote: Bool = true
    ) -> CastOutcome {
        CastOutcome(
            choice: choice, status: status, tally: ProposalTally(yes: yes, no: no, voters: 8, needed: 5),
            undoUntil: undo ? now.addingTimeInterval(15) : nil,
            myVote: myVote
                ? ProposalMyVote(order: order, of: 8, yesOrder: 5, noOrder: 0, decided: status != .open) : nil,
            changed: changed)
    }

    func testTheStatusLineReadsVotedRepliesAndTimeLeft() {
        let line = ProposalVoteCopy.statusLine(
            tally: tally, replies: 6, expiresAt: now.addingTimeInterval(4 * 3600 + 120), now: now)
        XCTAssertEqual(line, "5 of 8 voted · 6 replies · ends in 4h")
    }

    func testTheStatusLineLeavesOutZeroRepliesAndSingularises() {
        let end = now.addingTimeInterval(23 * 3600)
        XCTAssertEqual(
            ProposalVoteCopy.statusLine(tally: tally, replies: 0, expiresAt: end, now: now),
            "5 of 8 voted · ends in 23h")
        XCTAssertEqual(
            ProposalVoteCopy.statusLine(tally: tally, replies: 1, expiresAt: end, now: now),
            "5 of 8 voted · 1 reply · ends in 23h")
    }

    func testTheStatusLineCountsMinutesUnderAnHourAndDaysPastTwo() {
        XCTAssertEqual(ProposalVoteCopy.endsIn(now.addingTimeInterval(40 * 60), now: now), "ends in 40m")
        XCTAssertEqual(ProposalVoteCopy.endsIn(now.addingTimeInterval(20), now: now), "ends in 1m")
        XCTAssertEqual(ProposalVoteCopy.endsIn(now.addingTimeInterval(3 * 86_400), now: now), "ends in 3d")
        XCTAssertEqual(ProposalVoteCopy.endsIn(now.addingTimeInterval(-1), now: now), "voting closed")
    }

    func testTheHintsForPickedAndHolding() {
        XCTAssertEqual(ProposalVoteCopy.hint(.picked), "Hold to vote · lift to cancel")
        XCTAssertEqual(ProposalVoteCopy.hint(.holding), "Keep holding · lift to cancel")
    }

    func testAnOrdinaryCommitReadsYouVotedWithTheCounts() {
        let line = ProposalVoteCopy.committedOrdinary(choice: .yes, tally: tally)
        XCTAssertEqual(line.title, "You voted Yes")
        XCTAssertEqual(line.yes, 4)
        XCTAssertEqual(line.no, 1)
        XCTAssertEqual(line.accessibilityLabel, "You voted Yes. 4 yes, 1 no.")
        XCTAssertEqual(ProposalVoteCopy.committedOrdinary(choice: .no, tally: tally).title, "You voted No")
    }

    func testTheSettledLineForAPassedBuyAndASell() {
        let passed = ProposalTally(yes: 5, no: 1, voters: 8, needed: 5)
        XCTAssertEqual(
            ProposalVoteCopy.settledLine(
                status: .passed, tally: passed, voted: 6, voters: 8, sharesEstimate: "1.82"),
            "Passed 5–1 · 6 of 8 voted · buying 1.82 sh")
        XCTAssertEqual(
            ProposalVoteCopy.settledLine(
                status: .passed, tally: passed, voted: 6, voters: 8, sharesEstimate: "2.2", isSell: true),
            "Passed 5–1 · 6 of 8 voted · selling 2.2 sh")
        XCTAssertEqual(
            ProposalVoteCopy.settledLine(status: .passed, tally: passed, voted: 6, voters: 8, sharesEstimate: nil),
            "Passed 5–1 · 6 of 8 voted")
    }

    func testTheSettledLineKeepsTheApproximateSignAndNamesTokensForPreIpo() {
        let passed = ProposalTally(yes: 5, no: 1, voters: 8, needed: 5)
        XCTAssertEqual(
            ProposalVoteCopy.settledLine(
                status: .passed, tally: passed, voted: 6, voters: 8, sharesEstimate: "≈ 1.82"),
            "Passed 5–1 · 6 of 8 voted · buying ≈ 1.82 sh")
        XCTAssertEqual(
            ProposalVoteCopy.settledLine(
                status: .passed, tally: passed, voted: 6, voters: 8, sharesEstimate: "2.2", isSell: true,
                unit: "tokens"),
            "Passed 5–1 · 6 of 8 voted · selling 2.2 tokens")
    }

    func testTheSettledLineForAFailedProposalHasNoTrade() {
        let failed = ProposalTally(yes: 2, no: 3, voters: 5, needed: 3)
        XCTAssertEqual(
            ProposalVoteCopy.settledLine(status: .failed, tally: failed, voted: 5, voters: 5, sharesEstimate: "1.82"),
            "Failed 2–3 · 5 of 5 voted · no trade")
    }

    func testTheDecidingLineNamesTheOrderOfTheYesOrTheNo() {
        let yes = ProposalMyVote(order: 6, of: 8, yesOrder: 5, noOrder: 0, decided: true)
        XCTAssertEqual(
            ProposalVoteCopy.decidingLine(myVote: yes, needed: 5, voters: 8),
            "Yours was the 5th yes · at 5 of 8 it passes")
        let no = ProposalMyVote(order: 5, of: 5, yesOrder: 0, noOrder: 3, decided: true)
        XCTAssertEqual(
            ProposalVoteCopy.decidingLine(myVote: no, needed: 3, voters: 5), "Yours was the 3rd no · it can't pass now")
        let ordinary = ProposalMyVote(order: 6, of: 8, yesOrder: 4, noOrder: 0, decided: false)
        XCTAssertNil(ProposalVoteCopy.decidingLine(myVote: ordinary, needed: 5, voters: 8))
    }

    func testTheReadOnlyLabels() {
        XCTAssertEqual(ProposalVoteCopy.readOnlyLabel(cabalName: "Pilates"), "Pilates members vote")
        XCTAssertEqual(
            ProposalVoteCopy.readOnlyVoterLabel(cabalName: "Pilates"), "Voters from when it opened vote")
        XCTAssertEqual(
            ProposalVoteCopy.readOnlyAccessibilityLabel(
                tally: ProposalTally(yes: 3, no: 0, voters: 7, needed: 4), middle: "Pilates members vote"),
            "3 yes, 0 no. Pilates members vote. 3 of 7 voted.")
    }

    func testTheOrdinaryToastCarriesTheOrdinalFromTheVoteResponseAndUndo() {
        XCTAssertEqual(
            ProposalVoteCopy.toast(outcome: outcome()), VoteToast(message: "Voted Yes. 6th of 8.", offersUndo: true))
        XCTAssertEqual(
            ProposalVoteCopy.toast(outcome: outcome(.no, yes: 4, no: 2, order: 1)),
            VoteToast(message: "Voted No. 1st of 8.", offersUndo: true))
        XCTAssertEqual(
            ProposalVoteCopy.toast(outcome: outcome(myVote: false)), VoteToast(message: "Voted Yes.", offersUndo: true))
    }

    func testTheSettlingToastHasNoUndo() {
        XCTAssertEqual(
            ProposalVoteCopy.toast(outcome: outcome(status: .passed, undo: false)),
            VoteToast(message: "Voted Yes. Passed 5–1.", offersUndo: false))
        XCTAssertEqual(
            ProposalVoteCopy.toast(outcome: outcome(.no, status: .failed, yes: 2, no: 3, undo: false)),
            VoteToast(message: "Voted No. Failed 2–3.", offersUndo: false))
    }

    func testAChangeToastHasNoUndoAndTheUndoneToastIsFixed() {
        XCTAssertEqual(
            ProposalVoteCopy.toast(outcome: outcome(.no, undo: false, changed: true)),
            VoteToast(message: "Changed to No.", offersUndo: false))
        XCTAssertEqual(ProposalVoteCopy.undone, "Vote undone.")
    }

    func testOrdinalsOneToTwentyFour() {
        let expected = [
            "1st", "2nd", "3rd", "4th", "5th", "6th", "7th", "8th", "9th", "10th", "11th", "12th", "13th", "14th",
            "15th", "16th", "17th", "18th", "19th", "20th", "21st", "22nd", "23rd", "24th",
        ]
        XCTAssertEqual((1...24).map(ProposalVoteCopy.ordinal), expected)
    }

    func testOrdinalsPastAHundredKeepTheTeens() {
        XCTAssertEqual(ProposalVoteCopy.ordinal(101), "101st")
        XCTAssertEqual(ProposalVoteCopy.ordinal(111), "111th")
        XCTAssertEqual(ProposalVoteCopy.ordinal(112), "112th")
        XCTAssertEqual(ProposalVoteCopy.ordinal(122), "122nd")
    }
}
