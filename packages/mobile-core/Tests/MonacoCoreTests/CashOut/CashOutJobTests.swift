import MonacoAPI
import MonacoCore
import XCTest

final class CashOutJobTests: XCTestCase {
    private func job(_ status: CashOutJob.Status, resultCode: String? = nil) -> CashOutJob {
        CashOutJob(id: "j", cabalID: "c", status: status, payoutMicros: 1_000_000, resultCode: resultCode)
    }

    func testEveryRunningStatusShowsProgressAndNoOutcome() {
        XCTAssertEqual(job(.started).progress, "Cashing out…")
        XCTAssertEqual(job(.paying).progress, "Cashing out…")
        XCTAssertEqual(job(.selling).progress, "Selling your slice…")
        for status in [CashOutJob.Status.started, .selling, .paying] {
            XCTAssertTrue(job(status).isRunning)
            XCTAssertNil(job(status).outcome)
        }
    }

    func testEveryTerminalStatusShowsItsToast() {
        XCTAssertEqual(
            job(.completed).outcome,
            CashOutNotice(jobID: "j", message: "Cashed out $1.00. It's in your balance.", isSuccess: true))
        XCTAssertEqual(
            job(.partial).outcome,
            CashOutNotice(
                jobID: "j", message: "Cashed out $1.00. The sale came in short, so you kept part of your stake.",
                isSuccess: true))
        XCTAssertEqual(
            job(.failed, resultCode: "sale_short").outcome,
            CashOutNotice(
                jobID: "j",
                message: "Your cash out didn't go through. The sale fell short, so your stake stays in the cabal.",
                isSuccess: false))
        XCTAssertEqual(
            job(.failed, resultCode: "payout_failed").outcome?.message, "Your cash out didn't go through.")
        for status in [CashOutJob.Status.completed, .partial, .failed] {
            XCTAssertFalse(job(status).isRunning)
            XCTAssertNil(job(status).progress)
        }
    }

    func testTheStartedToastNamesTheJobsAmount() {
        XCTAssertEqual(job(.started).startedToast, "Cashing out $1.00. It lands in your balance in about a minute")
    }

    func testBothPauseRowsEndWithTheVotesLine() {
        let votes = "Passed votes won't trade until trading resumes."
        XCTAssertTrue(CabalPause(cause: .ops).message.hasSuffix(votes))
        XCTAssertTrue(CabalPause(cause: .externalDeposit).message.hasSuffix(votes))
        XCTAssertTrue(CabalPause(cause: .ops).message.hasPrefix("Trading is paused by Monaco."))
        XCTAssertTrue(
            CabalPause(cause: .externalDeposit).message.contains("sent money straight to this cabal's treasury"))
    }
}
