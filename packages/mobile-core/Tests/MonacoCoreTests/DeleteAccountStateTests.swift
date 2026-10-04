import MonacoAPI
import MonacoCore
import XCTest

final class DeleteAccountStateTests: XCTestCase {
    func testAStakeInACabalAsksToCashOutFirst() {
        let blocker = DeleteAccountState.state(for: problem(.accountHasPositions))
        XCTAssertEqual(blocker, .cashOutFirst)
        XCTAssertEqual(blocker?.message, "Cash out of every cabal first.")
    }

    func testABalanceAsksToWithdrawFirst() {
        let blocker = DeleteAccountState.state(for: problem(.accountHasBalance))
        XCTAssertEqual(blocker, .withdrawFirst)
        XCTAssertEqual(blocker?.message, "Withdraw your balance first.")
    }

    func testAnyOtherAnswerIsNoBlocker() {
        XCTAssertNil(DeleteAccountState.state(for: nil))
        XCTAssertNil(DeleteAccountState.state(for: problem(.dbUnavailable)))
        XCTAssertNil(DeleteAccountState.state(for: .transport(URLError(.notConnectedToInternet))))
        XCTAssertNil(DeleteAccountState.state(for: .accountDeleted))
    }

    func testTheDeleteAccountCopyPassesTheCopyAudit() {
        XCTAssertTrue(MainFlowCopyAudit.stringsAreClean(AccountCopy.auditedStrings))
        XCTAssertTrue(MainFlowCopyManifest.mainFlowStrings.contains(AccountCopy.explainer))
    }

    private func problem(_ code: Components.Schemas.ErrorCode) -> APIError {
        .problem(
            ProblemError(status: 409, code: .known(code), message: "Server says no.", traceID: "t", retryable: false))
    }
}
