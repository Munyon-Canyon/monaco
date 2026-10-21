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
            granted, .json(.accepted, accepted), .json(.ok, read("submitted")), .failure(URLError(.timedOut)),
            .failure(URLError(.notConnectedToInternet)), .json(.ok, read("confirmed")),
        ])
        let hints = FakeHintStream()
        let clock = TestClock()
        let withdrawing = Withdrawing(
            source: WithdrawSource(api: api(transport)), hints: hints, schedule: .standard(on: clock))
        _ = await withdrawing.submit(micros: 2_000_000, toAddress: address)

        let settled = Task { await withdrawing.settle() }
        await hints.waitForSubscribers(1)
        await transport.waitForRequests(3)
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
            granted, .json(.accepted, accepted), .json(.ok, read("submitted")), .json(.ok, read("confirmed")),
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
            granted, .json(.accepted, accepted), .json(.ok, read("submitted")), .gate,
            .json(.ok, read("submitted")), .json(.ok, read("confirmed")),
        ])
        let hints = FakeHintStream()
        let counted = PullCountingHints(base: hints)
        let withdrawing = Withdrawing(
            source: WithdrawSource(api: api(transport)), hints: counted, schedule: .standard(on: TestClock()))
        _ = await withdrawing.submit(micros: 2_000_000, toAddress: address)

        let settled = Task { await withdrawing.settle() }
        await hints.waitForSubscribers(1)
        await transport.waitForRequests(3)
        await hints.send(.changed(.user(other), what: "balance_changed", id: "0"))
        await transport.waitForRequests(4)
        for index in 1...50 {
            await hints.send(.changed(.user(other), what: "balance_changed", id: "\(index)"))
        }
        await counted.pulls.waitFor(52)
        await transport.releaseGate(.json(.ok, read("submitted")))
        await transport.waitForRequests(5)
        for _ in 0..<200 { await Task.yield() }

        let reads = await transport.sent.filter { $0.method == .get }.count
        XCTAssertEqual(reads, 3)
        await hints.send(.changed(.user(other), what: "balance_changed", id: "last"))
        let progress = await settled.value
        XCTAssertEqual(progress, .confirmed(withdrawal(.confirmed)))
    }

    private actor PullCount {
        private var count = 0
        private var waiters: [(count: Int, continuation: CheckedContinuation<Void, Never>)] = []

        func record() {
            count += 1
            let ready = waiters.filter { count >= $0.count }
            waiters.removeAll { count >= $0.count }
            for waiter in ready { waiter.continuation.resume() }
        }

        func waitFor(_ target: Int) async {
            if count >= target { return }
            await withCheckedContinuation { waiters.append((target, $0)) }
        }
    }

    private actor Relay {
        private var upstream: AsyncStream<Hint>.Iterator?
        private let pulls: PullCount

        init(_ stream: AsyncStream<Hint>, pulls: PullCount) {
            upstream = stream.makeAsyncIterator()
            self.pulls = pulls
        }

        func next() async -> Hint? {
            await pulls.record()
            guard var iterator = upstream else { return nil }
            upstream = nil
            let hint = await iterator.next(isolation: self)
            upstream = iterator
            return hint
        }
    }

    private struct PullCountingHints: HintSource {
        let base: FakeHintStream
        let pulls = PullCount()

        func hints(matching filter: HintFilter) -> AsyncStream<Hint> {
            let relay = Relay(base.hints(matching: filter), pulls: pulls)
            return AsyncStream { await relay.next() }
        }
    }

    private var granted: StubTransport.Reply {
        .json(
            .created,
            #"{"authorization_id":"01890a5d-ac96-774b-bcce-b302099a8059","expires_at":"2026-10-21T12:05:00Z"}"#)
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
