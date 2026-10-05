import Foundation
import HTTPTypes
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class FundProgressTests: XCTestCase {
    private let cabalID = "01890a5d-ac96-774b-bcce-b302099a8059"
    private let id = "01890a5d-ac96-774b-bcce-b302099a8057"
    private let other = "01890a5d-ac96-774b-bcce-b302099a8058"

    private func transfer(
        _ status: FundTransferStatus, id: String? = nil, shareUnits: String? = nil, failCode: String? = nil
    ) -> FundTransfer {
        FundTransfer(
            transferID: id ?? self.id, status: status, amountMicros: 5_000_000, shareUnits: shareUnits,
            failCode: failCode)
    }

    func testSubmittingEndsSubmittedOrBackToIdle() {
        XCTAssertEqual(FundProgress.idle.applying(.start), .submitting)
        XCTAssertEqual(
            FundProgress.submitting.applying(.accepted(transfer(.submitted))), .submitted(transfer(.submitted)))
        XCTAssertEqual(FundProgress.submitting.applying(.refused), .idle)
        XCTAssertEqual(FundProgress.submitting.applying(.start), .submitting)
    }

    func testAReadSettlesOrFailsTheSubmittedTransfer() {
        let sent = FundProgress.submitted(transfer(.submitted))
        XCTAssertEqual(sent.applying(.read(transfer(.landed))), sent)
        XCTAssertEqual(
            sent.applying(.read(transfer(.settled, shareUnits: "5000000"))),
            .settled(transfer(.settled, shareUnits: "5000000")))
        XCTAssertEqual(
            sent.applying(.read(transfer(.failed, failCode: "fund_not_sent"))), .failed(code: "fund_not_sent"))
    }

    func testAnotherTransfersReadChangesNothing() {
        let sent = FundProgress.submitted(transfer(.submitted))
        XCTAssertEqual(sent.applying(.read(transfer(.settled, id: other))), sent)
        XCTAssertEqual(FundProgress.idle.applying(.read(transfer(.settled))), .idle)
        let done = FundProgress.settled(transfer(.settled))
        XCTAssertEqual(done.applying(.read(transfer(.failed))), done)
    }

    func testTheToastsNameTheAmountAndTheCabal() {
        XCTAssertNil(FundProgress.idle.toast(cabalName: "Sunday Investors"))
        XCTAssertNil(FundProgress.submitting.toast(cabalName: "Sunday Investors"))
        XCTAssertEqual(
            FundProgress.submitted(transfer(.submitted)).toast(cabalName: "Sunday Investors"), "Funding this cabal…")
        XCTAssertEqual(
            FundProgress.settled(transfer(.settled)).toast(cabalName: "Sunday Investors"),
            "Added $5 to Sunday Investors.")
        let cents = FundTransfer(transferID: id, status: .settled, amountMicros: 1_234_560_000)
        XCTAssertEqual(FundProgress.settled(cents).toast(cabalName: "QA pot"), "Added $1,234.56 to QA pot.")
        XCTAssertEqual(
            FundProgress.failed(code: "fund_not_sent").toast(cabalName: "Sunday Investors"),
            "Funding didn't go through. Your balance wasn't charged.")
    }

    func testSubmitPostsTheAmountToTheCabalUnderAKey() async throws {
        let transport = StubTransport(.json(.accepted, accepted))
        let funding = funding(transport)

        let attempt = await funding.submit(micros: 5_000_000)

        XCTAssertEqual(attempt, .accepted(transfer(.submitted)))
        XCTAssertEqual(funding.progress, .submitted(transfer(.submitted)))
        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.method), [.post])
        XCTAssertEqual(sent.first?.path, "/v1/cabals/\(cabalID)/fund")
        XCTAssertNotNil(sent.first?.headerFields[try keyHeader()])
        let bodies = await transport.sentBodies
        let body = try XCTUnwrap(bodies.first ?? nil)
        let json = try XCTUnwrap(JSONSerialization.jsonObject(with: body) as? [String: String])
        XCTAssertEqual(json, ["amount_micros": "5000000"])
    }

    func testARefusalReturnsTheCodesCopyAndGoesBackToIdle() async throws {
        let transport = try StubTransport.problem(problem(422, .insufficientFunds))
        let funding = funding(transport)

        let attempt = await funding.submit(micros: 5_000_000)

        XCTAssertEqual(attempt, .refused(.needsMoney("Not enough in your account balance.")))
        XCTAssertEqual(funding.progress, .idle)
    }

    func testAPausedCabalRefusesWithThePausedCopy() async throws {
        let funding = funding(try StubTransport.problem(problem(409, .cabalPaused)))

        let attempt = await funding.submit(micros: 5_000_000)

        XCTAssertEqual(
            attempt, .refused(.toast("Trading in this cabal is paused. You can fund it again once it resumes.")))
    }

    func testAnUnknownOutcomeResendsTheSameKey() async throws {
        let transport = StubTransport(scripted: [.failure(URLError(.timedOut)), .json(.accepted, accepted)])
        let funding = funding(transport)

        let first = await funding.submit(micros: 5_000_000)
        guard case .unconfirmed = first else { return XCTFail("\(first)") }
        let second = await funding.submit(micros: 5_000_000)

        XCTAssertEqual(second, .accepted(transfer(.submitted)))
        let header = try keyHeader()
        let keys = await transport.sent.map { $0.headerFields[header] }
        XCTAssertEqual(keys.count, 2)
        XCTAssertEqual(keys[0], keys[1])
    }

    func testInFlightRetriesWithTheSameKey() async throws {
        let transport = StubTransport(scripted: [
            try .problem(problem(409, .idempotencyInFlight)), .json(.accepted, accepted),
        ])
        let funding = funding(transport)

        let attempt = await funding.submit(micros: 5_000_000)

        XCTAssertEqual(attempt, .accepted(transfer(.submitted)))
        let header = try keyHeader()
        let keys = await transport.sent.map { $0.headerFields[header] }
        XCTAssertEqual(keys.count, 2)
        XCTAssertEqual(keys[0], keys[1])
    }

    func testAHintBeforeThe202IsCaughtByTheFirstRead() async {
        let transport = StubTransport(scripted: [.json(.accepted, accepted), .json(.ok, read("settled"))])
        let hints = FakeHintStream()
        let funding = funding(transport, hints: hints)
        await hints.send(.changed(.cabal(cabalID), what: "activity_changed", id: "1"))
        _ = await funding.submit(micros: 5_000_000)

        let progress = await funding.settle()

        XCTAssertEqual(progress, .settled(transfer(.settled, shareUnits: "5000000")))
        let paths = await transport.sent.map(\.path)
        XCTAssertEqual(paths.last, "/v1/fund-transfers/\(id)")
    }

    func testSettleReadsOnCabalActivityAndResyncUntilItSettles() async {
        let transport = StubTransport(scripted: [
            .json(.accepted, accepted), .json(.ok, read("submitted")), .json(.ok, read("landed")),
            .json(.ok, read("settled")),
        ])
        let hints = FakeHintStream()
        let funding = funding(transport, hints: hints)
        _ = await funding.submit(micros: 5_000_000)

        let settled = Task { await funding.settle() }
        _ = await waitUntil {
            let subscribed = await hints.subscriberCount == 2
            let reads = await transport.sent.count
            return subscribed && reads == 2
        }
        await hints.send(.changed(.cabal(other), what: "activity_changed", id: "1"))
        await hints.send(.changed(.cabal(cabalID), what: "activity_changed", id: "2"))
        _ = await waitUntil { await transport.sent.count == 3 }
        await hints.send(.resync)

        let progress = await settled.value
        XCTAssertEqual(progress, .settled(transfer(.settled, shareUnits: "5000000")))
        _ = await waitUntil { await hints.subscriberCount == 0 }
        let subscribers = await hints.subscriberCount
        XCTAssertEqual(subscribers, 0)
    }

    func testABalanceHintReadsAFailedTransfer() async {
        let transport = StubTransport(scripted: [
            .json(.accepted, accepted), .json(.ok, read("submitted")), .json(.ok, read("failed")),
        ])
        let hints = FakeHintStream()
        let funding = funding(transport, hints: hints)
        _ = await funding.submit(micros: 5_000_000)

        let settled = Task { await funding.settle() }
        _ = await waitUntil {
            let subscribed = await hints.subscriberCount == 2
            let reads = await transport.sent.count
            return subscribed && reads == 2
        }
        await hints.send(.changed(.user(other), what: "balance_changed", id: "1"))

        let progress = await settled.value
        XCTAssertEqual(progress, .failed(code: "fund_not_sent"))
        XCTAssertEqual(progress.toast(cabalName: nil), "Funding didn't go through. Your balance wasn't charged.")
    }

    func testTheSamplesDecode() {
        XCTAssertEqual(Components.Schemas.FundAccepted.sample.status, .submitted)
        XCTAssertEqual(Components.Schemas.FundTransfer.sample.shareUnits, "5000000")
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

    private func problem(_ status: Int, _ code: Components.Schemas.ErrorCode) -> Components.Schemas.Problem {
        Components.Schemas.Problem(
            _type: .about_colon_blank, title: "Refused", status: status, code: code, message: "Server says no.",
            traceId: "4bf92f3577b34da6a3ce929d0e0e4736", retryable: false)
    }

    private func keyHeader() throws -> HTTPField.Name {
        try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
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
