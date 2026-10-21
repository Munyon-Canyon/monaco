import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class FundOutcomeTests: XCTestCase {
    private let cabalID = "01890a5d-ac96-774b-bcce-b302099a8059"
    private let id = "01890a5d-ac96-774b-bcce-b302099a8057"
    private let other = "01890a5d-ac96-774b-bcce-b302099a8058"

    private func transfer(_ status: FundTransferStatus) -> FundTransfer {
        FundTransfer(transferID: id, status: status, amountMicros: 5_000_000)
    }

    func testNoOutcomeBeforeTheServerAnswers() {
        XCTAssertNil(FundProgress.idle.outcome(requestedMicros: 5_000_000, cabalName: "Sunday Investors"))
        XCTAssertNil(FundProgress.submitting.outcome(requestedMicros: 5_000_000, cabalName: "Sunday Investors"))
    }

    func testSubmittedAndLandedAreOnlyPendingNeverConfirmed() {
        let sent = FundProgress.submitted(transfer(.submitted))
        for progress in [sent, sent.applying(.read(transfer(.landed))), sent.applying(.read(transfer(.submitted)))] {
            let outcome = progress.outcome(requestedMicros: 5_000_000, cabalName: "Sunday Investors")
            XCTAssertEqual(outcome?.phase, .pending)
            XCTAssertEqual(outcome?.title, "Funding this cabal…")
            XCTAssertEqual(
                outcome?.message, "$5.00 is on its way to Sunday Investors. Your share updates once funding settles.")
            XCTAssertNotEqual(outcome?.title, FundOutcome.settledTitle)
            XCTAssertFalse(outcome?.message.contains("added") ?? true)
        }
    }

    func testOnlySettledSaysTheShareWasAdded() {
        let outcome = FundProgress.settled(transfer(.settled)).outcome(
            requestedMicros: 5_000_000, cabalName: "Sunday Investors")

        XCTAssertEqual(outcome?.phase, .settled)
        XCTAssertEqual(outcome?.title, "Funding confirmed")
        XCTAssertEqual(outcome?.message, "$5.00 added to your share in Sunday Investors")
        XCTAssertEqual(
            FundProgress.settled(transfer(.settled)).outcome(requestedMicros: 5_000_000, cabalName: nil)?.message,
            "$5.00 added to your share in your cabal")
    }

    func testAFailureNamesTheAmountAndThatNothingWasCharged() {
        let outcome = FundProgress.failed(code: "fund_not_sent").outcome(
            requestedMicros: 5_000_000, cabalName: "Sunday Investors")

        XCTAssertEqual(outcome?.phase, .failed)
        XCTAssertEqual(outcome?.title, "Funding didn't go through")
        XCTAssertEqual(outcome?.message, "$5.00 to Sunday Investors wasn't sent. Your balance wasn't charged.")
    }

    func testAcceptedThenFailedShowsPendingFirstAndNeverASuccess() async {
        let transport = StubTransport(scripted: [
            .json(.accepted, accepted), .json(.ok, read("submitted")), .json(.ok, read("failed")),
        ])
        let hints = FakeHintStream()
        let funding = funding(transport, hints: hints)

        let attempt = await funding.submit(micros: 5_000_000)
        guard case .accepted = attempt else { return XCTFail("\(attempt)") }
        XCTAssertEqual(funding.progress.outcome(requestedMicros: 5_000_000, cabalName: nil)?.phase, .pending)
        let settled = Task { await funding.settle() }
        _ = await waitUntil {
            let subscribed = await hints.subscriberCount == 2
            let reads = await transport.sent.count
            return subscribed && reads == 2
        }
        XCTAssertEqual(funding.progress.outcome(requestedMicros: 5_000_000, cabalName: nil)?.phase, .pending)
        await hints.send(.changed(.user(other), what: "balance_changed", id: "1"))

        let progress = await settled.value
        XCTAssertEqual(progress.outcome(requestedMicros: 5_000_000, cabalName: nil)?.phase, .failed)
        XCTAssertNotEqual(progress.outcome(requestedMicros: 5_000_000, cabalName: nil)?.title, "Funding confirmed")
    }

    func testAcceptedThenSettledShowsPendingThenConfirmed() async {
        let transport = StubTransport(scripted: [.json(.accepted, accepted), .json(.ok, read("settled"))])
        let funding = funding(transport)

        _ = await funding.submit(micros: 5_000_000)
        XCTAssertEqual(funding.progress.outcome(requestedMicros: 5_000_000, cabalName: nil)?.phase, .pending)
        let progress = await funding.settle()

        let outcome = progress.outcome(requestedMicros: 5_000_000, cabalName: "Sunday Investors")
        XCTAssertEqual(outcome?.phase, .settled)
        XCTAssertEqual(outcome?.title, "Funding confirmed")
    }

    func testAnUnknownResponseShowsNoOutcomeAtAll() async {
        let funding = funding(StubTransport(scripted: [.failure(URLError(.timedOut))]))

        let attempt = await funding.submit(micros: 5_000_000)

        guard case .unconfirmed = attempt else { return XCTFail("\(attempt)") }
        XCTAssertNil(funding.progress.outcome(requestedMicros: 5_000_000, cabalName: nil))
    }

    func testLeavingBeforeItSettlesKeepsThePendingOutcome() async {
        let transport = StubTransport(scripted: [.json(.accepted, accepted), .json(.ok, read("landed"))])
        let hints = FakeHintStream()
        let funding = funding(transport, hints: hints)
        _ = await funding.submit(micros: 5_000_000)

        let settled = Task { await funding.settle() }
        _ = await waitUntil {
            let subscribed = await hints.subscriberCount == 2
            let reads = await transport.sent.count
            return subscribed && reads == 2
        }
        settled.cancel()
        let progress = await settled.value

        XCTAssertEqual(progress.outcome(requestedMicros: 5_000_000, cabalName: nil)?.phase, .pending)
        XCTAssertEqual(progress.toast(cabalName: "Sunday Investors"), "Funding this cabal…")
        XCTAssertFalse(progress.isSettled)
    }

    private var accepted: String {
        #"{"transfer_id":"\#(id)","status":"submitted"}"#
    }

    private func read(_ status: String) -> String {
        let shareUnits = status == "settled" ? #""5000000""# : "null"
        let failCode = status == "failed" ? #""fund_not_sent""# : "null"
        return #"{"status":"\#(status)","amount_micros":"5000000","share_units":\#(shareUnits),"#
            + #""fail_code":\#(failCode)}"#
    }

    private func funding(_ transport: StubTransport, hints: FakeHintStream = FakeHintStream()) -> Funding {
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        return Funding(cabalID: cabalID, source: FundSource(api: api), hints: hints, inFlightRetryDelay: .zero)
    }

    private func waitUntil(_ predicate: @escaping () async -> Bool) async -> Bool {
        for _ in 0..<500 {
            if await predicate() { return true }
            await Task.yield()
        }
        return await predicate()
    }
}
