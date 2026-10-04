import MonacoAPI
import MonacoCore
import XCTest

final class LeaveOutcomeTests: XCTestCase {
    func testHoldingSharesRoutesToCashOutWithTheServerMessage() {
        let outcome = LeaveOutcome(refusal: problem(.leaveHoldsShares, "Cash out your stake before you leave."))

        XCTAssertEqual(outcome, .cashOutFirst(message: "Cash out your stake before you leave."))
    }

    func testLastMemberWithMoneyInThePotStaysWithTheServerMessage() {
        let outcome = LeaveOutcome(refusal: problem(.leaveLastMemberPotNotEmpty, "The pot still holds money."))

        XCTAssertEqual(outcome, .refused(message: "The pot still holds money."))
    }

    func testCreatorWithMembersStaysWithTheServerMessage() {
        let outcome = LeaveOutcome(refusal: problem(.leaveCreatorWithMembers, "Everyone else has to leave first."))

        XCTAssertEqual(outcome, .refused(message: "Everyone else has to leave first."))
    }

    func testAnUnrelatedCodeFallsBackToToastCopy() {
        let error = problem(.upstreamUnavailable, "A service we depend on is down. Try again.")

        XCTAssertEqual(LeaveOutcome(refusal: error), .refused(message: ToastCopy.message(for: error)))
        XCTAssertEqual(
            LeaveOutcome(refusal: .transport(URLError(.notConnectedToInternet))),
            .refused(message: "You're offline. Try again.")
        )
    }

    private func problem(_ code: Components.Schemas.ErrorCode, _ message: String) -> APIError {
        .problem(ProblemError(status: 409, code: .known(code), message: message, traceID: "trace", retryable: false))
    }
}
