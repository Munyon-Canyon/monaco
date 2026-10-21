import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestClock
import MonacoTestSupport
import XCTest

@MainActor
final class WithdrawReconcileTests: XCTestCase {
    private let id = "01890a5d-ac96-774b-bcce-b302099a8057"
    private let other = "01890a5d-ac96-774b-bcce-b302099a8058"
    private let address = "9xQeWvG816bUx9EPjHmaT23yvVMvM9fQj4a8PHF4H6P"
    private let signature = "5VERv8NMvzbJMEkV8xnrLkEaWRtSz9CosKDYjCJjBRnbJLgp8uirBgmQpjKhoR4tjF3ZpRzrFmBV6UjKdiSZkQUW"

    private func withdrawal(_ status: WithdrawalStatus) -> Withdrawal {
        Withdrawal(withdrawalID: id, status: status, amountMicros: 2_000_000, txSignature: signature)
    }

    func testAFinalHintWhoseReadFailsStillConfirmsOnTheRetryWithNoMoreHints() async {
        let transport = StubTransport(scripted: [
            .json(.accepted, accepted), .json(.ok, read("submitted")), .failure(URLError(.timedOut)),
            .failure(URLError(.notConnectedToInternet)), .json(.ok, read("confirmed")),
        ])
        let hints = FakeHintStream()
        let clock = TestClock()
        let withdrawing = Withdrawing(
            source: WithdrawSource(api: api(transport)), hints: hints, schedule: .standard(on: clock))
        _ = await withdrawing.submit(micros: 2_000_000, toAddress: address)

        let settled = Task { await withdrawing.settle() }
        await hints.waitForSubscribers(1)
        await transport.waitForRequests(2)
        await hints.send(.changed(.user(other), what: "balance_changed", id: "1"))
        _ = await clock.state.until { $0.requested.contains(.seconds(1)) }
        clock.advance(by: .seconds(1))
        _ = await clock.state.until { $0.requested.contains(.seconds(2)) }
        clock.advance(by: .seconds(2))

        let progress = await settled.value
        XCTAssertEqual(progress, .confirmed(withdrawal(.confirmed)))
        let reads = await transport.sent.filter { $0.method == .get }.count
        XCTAssertEqual(reads, 4)
    }

    func testAnUnresolvedWithdrawalIsReadAgainOnTheIntervalWithNoHints() async {
        let transport = StubTransport(scripted: [
            .json(.accepted, accepted), .json(.ok, read("submitted")), .json(.ok, read("confirmed")),
        ])
        let clock = TestClock()
        let withdrawing = Withdrawing(
            source: WithdrawSource(api: api(transport)), hints: FakeHintStream(), schedule: .standard(on: clock))
        _ = await withdrawing.submit(micros: 2_000_000, toAddress: address)

        let settled = Task { await withdrawing.settle() }
        _ = await clock.state.until { $0.requested.contains(ReconcileSchedule.standard().interval) }
        clock.advance(by: ReconcileSchedule.standard().interval)

        let progress = await settled.value
        XCTAssertEqual(progress, .confirmed(withdrawal(.confirmed)))
    }

    func testABurstOfHintsDuringASlowReadQueuesOneMoreRead() async {
        let transport = StubTransport(scripted: [
            .json(.accepted, accepted), .json(.ok, read("submitted")), .gate, .json(.ok, read("submitted")),
            .json(.ok, read("confirmed")),
        ])
        let hints = FakeHintStream()
        let withdrawing = Withdrawing(
            source: WithdrawSource(api: api(transport)), hints: hints, schedule: .standard(on: TestClock()))
        _ = await withdrawing.submit(micros: 2_000_000, toAddress: address)

        let settled = Task { await withdrawing.settle() }
        await hints.waitForSubscribers(1)
        await transport.waitForRequests(2)
        await hints.send(.changed(.user(other), what: "balance_changed", id: "0"))
        await transport.waitForRequests(3)
        for index in 1...50 {
            await hints.send(.changed(.user(other), what: "balance_changed", id: "\(index)"))
        }
        await transport.releaseGate(.json(.ok, read("submitted")))
        await transport.waitForRequests(4)
        for _ in 0..<200 { await Task.yield() }

        let reads = await transport.sent.filter { $0.method == .get }.count
        XCTAssertEqual(reads, 3)
        await hints.send(.changed(.user(other), what: "balance_changed", id: "last"))
        let progress = await settled.value
        XCTAssertEqual(progress, .confirmed(withdrawal(.confirmed)))
    }

    private var accepted: String {
        #"{"withdrawal_id":"\#(id)","status":"submitted","tx_signature":"\#(signature)"}"#
    }

    private func read(_ status: String) -> String {
        #"{"withdrawal_id":"\#(id)","status":"\#(status)","amount_micros":"2000000","#
            + #""to_address":"\#(address)","tx_signature":"\#(signature)","fail_code":null,"#
            + #""created_at":"2025-10-04T12:00:00Z","completed_at":null}"#
    }

    private func api(_ transport: StubTransport) -> APIClient {
        APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
    }
}
