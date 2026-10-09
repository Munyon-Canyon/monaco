import XCTest

@testable import MonacoCore

final class ProposalStepperTests: XCTestCase {
    private let closes = Date(timeIntervalSince1970: 1_800_000_000)

    private func make(
        _ status: ProposalStatus, isSell: Bool = false, swapFailed: Bool = false, failure: String? = nil,
        message: String? = nil
    ) -> ProposalStepper {
        ProposalStepper.make(
            status: status, isSell: isSell, swapFailed: swapFailed, expiresAt: closes, failureMessage: failure,
            statusMessage: message)
    }

    private func titles(_ stepper: ProposalStepper) -> [String] { stepper.steps.map(\.title) }
    private func marks(_ stepper: ProposalStepper) -> [ProposalStepper.Mark] { stepper.steps.map(\.mark) }

    func testOpenVotesThenBuysThenBought() {
        let stepper = make(.open)
        XCTAssertEqual(titles(stepper), ["Voting", "Buying", "Bought"])
        XCTAssertEqual(marks(stepper), [.active, .pending, .pending])
        XCTAssertEqual(stepper.steps[0].stamp, .init(prefix: "Closes", date: closes))
        XCTAssertFalse(stepper.isTerminal)
    }

    func testPassedIsBuyingNow() {
        let stepper = make(.passed)
        XCTAssertEqual(marks(stepper), [.done, .active, .pending])
        XCTAssertNil(stepper.steps[0].stamp)
    }

    func testASellUsesSellingAndSold() {
        XCTAssertEqual(titles(make(.passed, isSell: true)), ["Voting", "Selling", "Sold"])
    }

    func testExecutedIsAllDone() {
        let stepper = make(.executed)
        XCTAssertEqual(titles(stepper), ["Voting", "Buying", "Bought"])
        XCTAssertEqual(marks(stepper), [.done, .done, .done])
    }

    func testAFailedSwapShowsItsMessageOnceOnTheBuyingStep() {
        let stepper = make(.passed, swapFailed: true, failure: "Price moved too far", message: "ignored")
        XCTAssertEqual(titles(stepper), ["Voting", "Couldn't buy"])
        XCTAssertEqual(marks(stepper), [.done, .failed])
        XCTAssertEqual(stepper.steps.compactMap(\.note), ["Price moved too far. The money is still in the pot."])
    }

    func testAFailedSwapWithoutAMessageFallsBackToCouldntSell() {
        XCTAssertEqual(make(.passed, isSell: true, swapFailed: true).steps[1].note, "The shares are still in the pot.")
    }

    func testAFailedSellKeepsAMessageThatAlreadyEndsInAPeriod() {
        let stepper = make(.passed, isSell: true, swapFailed: true, failure: "Price moved too far.")
        XCTAssertEqual(stepper.steps[1].title, "Couldn't sell")
        XCTAssertEqual(stepper.steps[1].note, "Price moved too far. The shares are still in the pot.")
    }

    func testExecutionBlockedShowsTheStatusMessage() {
        let stepper = make(.executionBlocked, message: "The cabal is short on USDC.")
        XCTAssertEqual(marks(stepper), [.done, .failed])
        XCTAssertEqual(stepper.steps[1].title, "Couldn't buy")
        XCTAssertEqual(stepper.steps[1].note, "The cabal is short on USDC.")
    }

    func testExpiredIsOneTerminalRowWithNoTradeStep() {
        let stepper = make(.expired)
        XCTAssertTrue(stepper.isTerminal)
        XCTAssertEqual(titles(stepper), ["Expired"])
        XCTAssertEqual(stepper.steps[0].stamp, .init(prefix: "Voting closed", date: closes))
        XCTAssertEqual(stepper.steps[0].note, "Without enough yes votes.")
        XCTAssertEqual(stepper.accessibilityLabel.components(separatedBy: "Expired").count - 1, 1)
    }

    func testRejectedIsOneTerminalRowThatClosedVoting() {
        let stepper = make(.failed)
        XCTAssertEqual(titles(stepper), ["Didn't pass"])
        XCTAssertNotNil(stepper.steps[0].stamp)
        XCTAssertEqual(stepper.steps[0].note, "Not enough yes votes to pass.")
    }

    func testWithdrawnIsOneTerminalRowWithNoCloseTime() {
        let stepper = make(.withdrawn)
        XCTAssertEqual(titles(stepper), ["Withdrawn"])
        XCTAssertNil(stepper.steps[0].stamp)
        XCTAssertTrue(stepper.isTerminal)
    }

    func testVoidedIsOneTerminalRowCarryingTheMessage() {
        let stepper = make(.voided, message: "The asset was delisted.")
        XCTAssertEqual(titles(stepper), ["Voided by Monaco"])
        XCTAssertEqual(stepper.steps[0].note, "The asset was delisted.")
    }

    func testTheAccessibilityLabelNamesEachStepState() {
        XCTAssertEqual(
            make(.passed).accessibilityLabel,
            "Voting, step 1 of 3, done. Buying, step 2 of 3, in progress. Bought, step 3 of 3, to do")
    }
}
