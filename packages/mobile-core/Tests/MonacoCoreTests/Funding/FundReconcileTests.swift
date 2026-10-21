import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestClock
import MonacoTestSupport
import XCTest

@MainActor
final class FundReconcileTests: XCTestCase {
    private let cabalID = "01890a5d-ac96-774b-bcce-b302099a8059"
    private let id = "01890a5d-ac96-774b-bcce-b302099a8057"
    private let other = "01890a5d-ac96-774b-bcce-b302099a8058"

    private func transfer(_ status: FundTransferStatus, shareUnits: String? = nil) -> FundTransfer {
        FundTransfer(transferID: id, status: status, amountMicros: 5_000_000, shareUnits: shareUnits)
    }

    func testAFinalHintWhoseReadFailsStillSettlesOnTheRetryWithNoMoreHints() async {
        let transport = StubTransport(scripted: [
            .json(.accepted, accepted), .json(.ok, read("submitted")), .failure(URLError(.timedOut)),
            .failure(URLError(.notConnectedToInternet)), .json(.ok, read("settled")),
        ])
        let hints = FakeHintStream()
        let clock = TestClock()
        let funding = funding(transport, hints: hints, clock: clock)
        _ = await funding.submit(micros: 5_000_000)

        let settled = Task { await funding.settle() }
        await hints.waitForSubscribers(2)
        await transport.waitForRequests(2)
        await hints.send(.changed(.cabal(cabalID), what: "activity_changed", id: "1"))
        _ = await clock.state.until { $0.requested.contains(.seconds(1)) }
        clock.advance(by: .seconds(1))
        _ = await clock.state.until { $0.requested.contains(.seconds(2)) }
        clock.advance(by: .seconds(2))

        let progress = await settled.value
        XCTAssertEqual(progress, .settled(transfer(.settled, shareUnits: "5000000")))
        let reads = await transport.sent.filter { $0.method == .get }.count
        XCTAssertEqual(reads, 4)
    }

    func testAnUnresolvedTransferIsReadAgainOnTheIntervalWithNoHints() async {
        let transport = StubTransport(scripted: [
            .json(.accepted, accepted), .json(.ok, read("submitted")), .json(.ok, read("landed")),
            .json(.ok, read("settled")),
        ])
        let clock = TestClock()
        let funding = funding(transport, clock: clock)
        _ = await funding.submit(micros: 5_000_000)

        let settled = Task { await funding.settle() }
        _ = await clock.state.until { $0.requested.contains(ReconcileSchedule.standard().interval) }
        clock.advance(by: ReconcileSchedule.standard().interval)
        _ = await clock.state.until { $0.requested.count == 2 }
        clock.advance(by: ReconcileSchedule.standard().interval)

        let progress = await settled.value
        XCTAssertEqual(progress, .settled(transfer(.settled, shareUnits: "5000000")))
    }

    func testTheRetryDelayDoublesAndStopsAtTheInterval() {
        let schedule = ReconcileSchedule.standard()
        let delays = (0...7).map { schedule.delay(afterFailures: $0) }
        let expected: [Duration] = [
            .seconds(15), .seconds(1), .seconds(2), .seconds(4), .seconds(8), .seconds(15), .seconds(15), .seconds(15),
        ]
        XCTAssertEqual(delays, expected)
    }

    func testABurstOfHintsDuringASlowReadQueuesOneMoreRead() async {
        let transport = StubTransport(scripted: [
            .json(.accepted, accepted), .json(.ok, read("submitted")), .gate, .json(.ok, read("submitted")),
            .json(.ok, read("settled")),
        ])
        let hints = FakeHintStream()
        let funding = funding(transport, hints: hints, clock: TestClock())
        _ = await funding.submit(micros: 5_000_000)

        let settled = Task { await funding.settle() }
        await hints.waitForSubscribers(2)
        await transport.waitForRequests(2)
        await hints.send(.changed(.cabal(cabalID), what: "activity_changed", id: "0"))
        await transport.waitForRequests(3)
        for index in 1...50 {
            await hints.send(.changed(.cabal(cabalID), what: "activity_changed", id: "\(index)"))
            await hints.send(.changed(.user(other), what: "balance_changed", id: "b\(index)"))
        }
        await transport.releaseGate(.json(.ok, read("submitted")))
        await transport.waitForRequests(4)
        for _ in 0..<200 { await Task.yield() }

        let reads = await transport.sent.filter { $0.method == .get }.count
        XCTAssertEqual(reads, 3)
        await hints.send(.changed(.cabal(cabalID), what: "activity_changed", id: "last"))
        let progress = await settled.value
        XCTAssertEqual(progress, .settled(transfer(.settled, shareUnits: "5000000")))
    }

    private var accepted: String {
        #"{"transfer_id":"\#(id)","status":"submitted"}"#
    }

    private func read(_ status: String) -> String {
        let shareUnits = status == "settled" ? #""5000000""# : "null"
        return #"{"status":"\#(status)","amount_micros":"5000000","share_units":\#(shareUnits),"fail_code":null}"#
    }

    private func funding(
        _ transport: StubTransport, hints: FakeHintStream = FakeHintStream(), clock: TestClock
    ) -> Funding {
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        return Funding(
            cabalID: cabalID, source: FundSource(api: api), hints: hints, inFlightRetryDelay: .zero,
            schedule: .standard(on: clock))
    }
}
