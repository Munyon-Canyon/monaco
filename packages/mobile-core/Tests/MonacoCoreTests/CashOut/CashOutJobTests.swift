import MonacoAPI
import MonacoCore
import XCTest

final class CashOutJobTests: XCTestCase {
    private func job(
        _ status: CashOutJob.Status, resultCode: String? = nil, payoutMicros: Int64 = 1_000_000
    ) -> CashOutJob {
        CashOutJob(id: "j", cabalID: "c", status: status, payoutMicros: payoutMicros, resultCode: resultCode)
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
                jobID: "j", message: "Cashed out $1.00, what the sale raised. You keep the shares it didn't cover.",
                isSuccess: true, raisedMicros: 1_000_000))
        XCTAssertEqual(
            job(.failed, resultCode: "sale_short").outcome,
            CashOutNotice(
                jobID: "j",
                message: "Your cash out didn't go through. The sale fell short, so your slice stays in the cabal.",
                isSuccess: false))
        XCTAssertEqual(
            job(.failed, resultCode: "payout_failed").outcome?.message, "Your cash out didn't go through.")
        for status in [CashOutJob.Status.completed, .partial, .failed] {
            XCTAssertFalse(job(status).isRunning)
            XCTAssertNil(job(status).progress)
        }
    }

    func testAPartialNoticeNamesWhatTheSaleRaisedAndWhatWasRequested() throws {
        let notice = try XCTUnwrap(job(.partial, payoutMicros: 180_000_000).outcome)

        XCTAssertTrue(notice.isPartial)
        XCTAssertEqual(
            notice.partialMessage(requestedMicros: 250_000_000),
            "Cashed out $180 of $250. You keep the shares it didn't cover.")
        XCTAssertFalse(try XCTUnwrap(job(.completed).outcome).isPartial)
        XCTAssertNil(try XCTUnwrap(job(.completed).outcome).partialMessage(requestedMicros: 250_000_000))
    }

    func testTheStartedToastNamesTheJobsAmount() {
        XCTAssertEqual(job(.started).startedToast, "Cashing out $1.00. The final amount is known when it settles.")
    }

    func testEveryToastFloorsThePayoutToTheCentsTheButtonNames() {
        let payout: Int64 = 10_505_000
        XCTAssertEqual(
            CashOutAmountRule.submitTitle(for: .ok, enteredMicros: payout, sliceMicros: 20_000_000), "Cash out $10.50")
        func job(_ status: CashOutJob.Status) -> CashOutJob {
            CashOutJob(id: "j", cabalID: "c", status: status, payoutMicros: payout, resultCode: nil)
        }
        XCTAssertEqual(job(.started).startedToast, "Cashing out $10.50. The final amount is known when it settles.")
        XCTAssertEqual(job(.completed).outcome?.message, "Cashed out $10.50. It's in your balance.")
        XCTAssertEqual(
            job(.partial).outcome?.message,
            "Cashed out $10.50, what the sale raised. You keep the shares it didn't cover.")
    }

    func testPauseRowsAreCompleteSentencesInProductWords() {
        XCTAssertEqual(
            CabalPause(cause: .externalDeposit).message,
            "Trading is paused. Someone sent money straight to this cabal, and it's being returned. "
                + "Funding, cash outs and trades resume once it's back.")
        XCTAssertEqual(
            CabalPause(cause: .ops).message,
            "Trading is paused by Monaco. Funding, cash outs and trades resume when the pause is lifted.")
        for cause in [CabalPause.Cause.ops, .externalDeposit] {
            XCTAssertFalse(CabalPause(cause: cause).message.lowercased().contains("treasury"))
        }
    }
}
