import Foundation
import HTTPTypes
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class WithdrawProgressTests: XCTestCase {
    private let id = "01890a5d-ac96-774b-bcce-b302099a8057"
    private let other = "01890a5d-ac96-774b-bcce-b302099a8058"
    private let address = "9xQeWvG816bUx9EPjHmaT23yvVMvM9fQj4a8PHF4H6P"
    private let otherAddress = "7EcDhSYGxXyscszYEp35KHN8vvw3svAuLKTzXwCFLtV"
    private let signature = "5VERv8NMvzbJMEkV8xnrLkEaWRtSz9CosKDYjCJjBRnbJLgp8uirBgmQpjKhoR4tjF3ZpRzrFmBV6UjKdiSZkQUW"

    private func withdrawal(_ status: WithdrawalStatus, id: String? = nil, failCode: String? = nil) -> Withdrawal {
        Withdrawal(
            withdrawalID: id ?? self.id, status: status, amountMicros: 2_000_000, txSignature: signature,
            failCode: failCode)
    }

    func testSubmittingEndsSubmittedOrBackToIdle() {
        XCTAssertEqual(WithdrawProgress.idle.applying(.start), .submitting)
        XCTAssertEqual(
            WithdrawProgress.submitting.applying(.accepted(withdrawal(.submitted))), .submitted(withdrawal(.submitted)))
        XCTAssertEqual(WithdrawProgress.submitting.applying(.refused), .idle)
        XCTAssertEqual(WithdrawProgress.submitting.applying(.start), .submitting)
    }

    func testAReadConfirmsOrFailsTheSubmittedWithdrawal() {
        let sent = WithdrawProgress.submitted(withdrawal(.submitted))
        XCTAssertEqual(sent.applying(.read(withdrawal(.submitted))), sent)
        XCTAssertEqual(sent.applying(.read(withdrawal(.confirmed))), .confirmed(withdrawal(.confirmed)))
        XCTAssertEqual(
            sent.applying(.read(withdrawal(.failed, failCode: "privy_unavailable"))),
            .failed(code: "privy_unavailable"))
    }

    func testAnotherWithdrawalsReadChangesNothing() {
        let sent = WithdrawProgress.submitted(withdrawal(.submitted))
        XCTAssertEqual(sent.applying(.read(withdrawal(.confirmed, id: other))), sent)
        XCTAssertEqual(WithdrawProgress.idle.applying(.read(withdrawal(.confirmed))), .idle)
    }

    func testASettledWithdrawalStaysSettled() {
        let done = WithdrawProgress.confirmed(withdrawal(.confirmed))
        XCTAssertEqual(done.applying(.read(withdrawal(.failed))), done)
        XCTAssertEqual(done.applying(.start), done)
        XCTAssertEqual(WithdrawProgress.failed(code: nil).applying(.read(withdrawal(.confirmed))), .failed(code: nil))
    }

    func testTheToastsSayWithdrawAndTheAmount() {
        XCTAssertNil(WithdrawProgress.idle.toast)
        XCTAssertNil(WithdrawProgress.submitting.toast)
        XCTAssertEqual(
            WithdrawProgress.submitted(withdrawal(.submitted)).toast,
            "Withdrawing $2.00. It lands in about a minute.")
        XCTAssertEqual(WithdrawProgress.confirmed(withdrawal(.confirmed)).toast, "Withdrawal complete: $2.00")
        XCTAssertEqual(
            WithdrawProgress.failed(code: "privy_unavailable").toast,
            "Withdrawal didn't go through. Your balance wasn't charged.")
        XCTAssertEqual(withdrawal(.confirmed).solscanURL?.absoluteString, "https://solscan.io/tx/\(signature)")
    }

    func testSubmitConfirmsTheExactWithdrawalThenPostsItUnderAKey() async throws {
        let transport = StubTransport(scripted: [granted, .json(.accepted, accepted)])
        let withdrawing = Withdrawing(source: WithdrawSource(api: api(transport)), hints: FakeHintStream())

        let attempt = await withdrawing.submit(micros: 2_000_000, toAddress: address)

        XCTAssertEqual(attempt, .accepted(withdrawal(.submitted)))
        XCTAssertEqual(withdrawing.progress, .submitted(withdrawal(.submitted)))
        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.method), [.post, .post])
        XCTAssertEqual(sent.map(\.path), ["/v1/me/withdrawals/authorizations", "/v1/me/withdrawals"])
        XCTAssertNil(sent[0].headerFields[try keyHeader()])
        XCTAssertNotNil(sent[1].headerFields[try keyHeader()])
        XCTAssertEqual(sent[1].headerFields[try authorizationHeader()], authorization)
        let bodies = await transport.sentBodies
        for sentBody in bodies {
            let body = try XCTUnwrap(sentBody)
            let json = try XCTUnwrap(JSONSerialization.jsonObject(with: body) as? [String: String])
            XCTAssertEqual(json, ["amount_micros": "2000000", "to_address": address])
        }
    }

    func testAnAuthorizationRefusalShowsTheServersLineAndSendsNothingElse() async throws {
        for (code, line) in [
            (Components.Schemas.ErrorCode.reauthRequired, "Sign in again to confirm this withdrawal."),
            (.stepUpMismatch, "The amount or address changed after you confirmed. Confirm the withdrawal again."),
            (.stepUpRequired, "Confirm this withdrawal to continue."),
        ] {
            let transport = try StubTransport.problem(problem(422, code, message: line))
            let withdrawing = withdrawing(transport)

            let attempt = await withdrawing.submit(micros: 2_000_000, toAddress: address)

            XCTAssertEqual(attempt, .refused(.toast(line)), "\(code)")
            XCTAssertEqual(withdrawing.progress, .idle)
            let paths = await transport.sent.map(\.path)
            XCTAssertEqual(
                paths, ["/v1/me/withdrawals/authorizations"], "nothing is sent once the confirmation is refused")
        }
    }

    func testAnAddressRefusalReturnsTheInlineCopy() async throws {
        let transport = try StubTransport.problem(problem(400, .withdrawToOwnWallet))
        let withdrawing = Withdrawing(source: WithdrawSource(api: api(transport)), hints: FakeHintStream())

        let attempt = await withdrawing.submit(micros: 2_000_000, toAddress: address)

        XCTAssertEqual(
            attempt,
            .refused(.address("That's your own deposit address. Paste the address you want to send to.")))
        XCTAssertEqual(withdrawing.progress, .idle)
    }

    func testAnUnknownOutcomeResendsTheSameKey() async throws {
        let transport = StubTransport(scripted: [
            granted, .failure(URLError(.timedOut)), granted, .json(.accepted, accepted),
        ])
        let withdrawing = Withdrawing(source: WithdrawSource(api: api(transport)), hints: FakeHintStream())

        let first = await withdrawing.submit(micros: 2_000_000, toAddress: address)
        guard case .unconfirmed = first else { return XCTFail("\(first)") }
        XCTAssertEqual(withdrawing.progress, .idle)
        let second = await withdrawing.submit(micros: 2_000_000, toAddress: address)

        XCTAssertEqual(second, .accepted(withdrawal(.submitted)))
        let keys = try await withdrawKeys(transport)
        XCTAssertEqual(keys.count, 2)
        XCTAssertEqual(keys[0], keys[1])
    }

    func testAnAnswerlessSubmitSaysToCheckTheBalanceFirst() async throws {
        let inFlight = try StubTransport.Reply.problem(problem(409, .idempotencyInFlight))
        let answerless: [[StubTransport.Reply]] = [
            [granted, .failure(URLError(.networkConnectionLost))], [granted, .json(.accepted, "{")],
            Array(repeating: [granted, inFlight], count: 4).flatMap { $0 },
        ]
        for replies in answerless {
            let withdrawing = withdrawing(StubTransport(scripted: replies))

            let attempt = await withdrawing.submit(micros: 2_000_000, toAddress: address)

            XCTAssertEqual(attempt, .unconfirmed(retryMessage: MoneyFlowCopy.unconfirmed.summary), "\(replies)")
            XCTAssertEqual(withdrawing.progress, .idle)
        }
    }

    func testAServerErrorKeepsTheServersLine() async throws {
        let withdrawing = withdrawing(try StubTransport.problem(problem(503, .privyUnavailable)))

        let attempt = await withdrawing.submit(micros: 2_000_000, toAddress: address)

        XCTAssertEqual(attempt, .unconfirmed(retryMessage: "Server says no."))
    }

    func testAChangedAmountOrAddressWhileUnconfirmedSendsNothingAndTheSamePayloadKeepsTheKey() async throws {
        let transport = StubTransport(scripted: [
            granted, .failure(URLError(.timedOut)), granted, .json(.accepted, accepted),
        ])
        let withdrawing = withdrawing(transport)
        _ = await withdrawing.submit(micros: 2_000_000, toAddress: address)

        let changedAmount = await withdrawing.submit(micros: 3_000_000, toAddress: address)
        let changedAddress = await withdrawing.submit(micros: 2_000_000, toAddress: otherAddress)

        XCTAssertEqual(changedAmount, .unconfirmed(retryMessage: MoneyFlowCopy.unconfirmed.summary))
        XCTAssertEqual(changedAddress, .unconfirmed(retryMessage: MoneyFlowCopy.unconfirmed.summary))
        let afterChange = await transport.sent.count
        XCTAssertEqual(afterChange, 2)
        let same = await withdrawing.submit(micros: 2_000_000, toAddress: address)
        XCTAssertEqual(same, .accepted(withdrawal(.submitted)))
        let keys = try await withdrawKeys(transport)
        XCTAssertEqual(keys.count, 2)
        XCTAssertEqual(keys[0], keys[1])
    }

    func testAFinalAnswerLetsADifferentAddressGoOut() async throws {
        let transport = StubTransport(scripted: [
            granted, .failure(URLError(.timedOut)), granted, try .problem(problem(400, .withdrawToOwnWallet)),
            granted, .json(.accepted, accepted),
        ])
        let withdrawing = withdrawing(transport)
        _ = await withdrawing.submit(micros: 2_000_000, toAddress: address)
        _ = await withdrawing.submit(micros: 2_000_000, toAddress: address)

        let corrected = await withdrawing.submit(micros: 2_000_000, toAddress: otherAddress)

        XCTAssertEqual(corrected, .accepted(withdrawal(.submitted)))
        let keys = try await withdrawKeys(transport)
        XCTAssertEqual(keys.count, 3)
        XCTAssertNotEqual(keys[1], keys[2])
    }

    func testInFlightRetriesWithTheSameKey() async throws {
        let transport = StubTransport(scripted: [
            granted, try .problem(problem(409, .idempotencyInFlight)), granted, .json(.accepted, accepted),
        ])
        let withdrawing = Withdrawing(
            source: WithdrawSource(api: api(transport)), hints: FakeHintStream(), inFlightRetryDelay: .zero)

        let attempt = await withdrawing.submit(micros: 2_000_000, toAddress: address)

        XCTAssertEqual(attempt, .accepted(withdrawal(.submitted)))
        let keys = try await withdrawKeys(transport)
        XCTAssertEqual(keys.count, 2)
        XCTAssertEqual(keys[0], keys[1])
    }

    func testSettleReadsOnEveryBalanceHintUntilItConfirms() async {
        let transport = StubTransport(scripted: [
            granted, .json(.accepted, accepted), .json(.ok, read("submitted")), .json(.ok, read("submitted")),
            .json(.ok, read("confirmed")),
        ])
        let hints = FakeHintStream()
        let withdrawing = Withdrawing(source: WithdrawSource(api: api(transport)), hints: hints)
        _ = await withdrawing.submit(micros: 2_000_000, toAddress: address)

        let settled = Task { await withdrawing.settle() }
        _ = await waitUntil {
            let subscribed = await hints.subscriberCount == 1
            let reads = await transport.sent.count
            return subscribed && reads == 3
        }
        await hints.send(.changed(.user(other), what: "balance_changed", id: "1"))
        _ = await waitUntil { await transport.sent.count == 4 }
        await hints.send(.resync)

        let progress = await settled.value
        XCTAssertEqual(progress, .confirmed(withdrawal(.confirmed)))
        let paths = await transport.sent.map(\.path)
        XCTAssertEqual(paths.last, "/v1/me/withdrawals/\(id)")
    }

    func testSettleEndsAtOnceWhenTheFirstReadFailed() async {
        let transport = StubTransport(scripted: [granted, .json(.accepted, accepted), .json(.ok, read("failed"))])
        let withdrawing = Withdrawing(source: WithdrawSource(api: api(transport)), hints: FakeHintStream())
        _ = await withdrawing.submit(micros: 2_000_000, toAddress: address)

        let progress = await withdrawing.settle()

        XCTAssertEqual(progress, .failed(code: "privy_unavailable"))
    }

    func testTheSamplesDecode() {
        XCTAssertEqual(Components.Schemas.WithdrawAccepted.sample.status, .submitted)
        XCTAssertEqual(Components.Schemas.Withdrawal.sample.amountMicros, "2000000")
    }

    private var accepted: String {
        #"{"withdrawal_id":"\#(id)","status":"submitted","tx_signature":"\#(signature)"}"#
    }

    private func read(_ status: String) -> String {
        let failCode = status == "failed" ? #""privy_unavailable""# : "null"
        return #"{"withdrawal_id":"\#(id)","status":"\#(status)","amount_micros":"2000000","#
            + #""to_address":"\#(address)","tx_signature":"\#(signature)","fail_code":\#(failCode),"#
            + #""created_at":"2025-10-04T12:00:00Z","completed_at":null}"#
    }

    private func problem(
        _ status: Int, _ code: Components.Schemas.ErrorCode, message: String = "Server says no."
    ) -> Components.Schemas.Problem {
        Components.Schemas.Problem(
            _type: .about_colon_blank, title: "Refused", status: status, code: code, message: message,
            traceId: "4bf92f3577b34da6a3ce929d0e0e4736", retryable: false)
    }

    private func keyHeader() throws -> HTTPField.Name {
        try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
    }

    private func authorizationHeader() throws -> HTTPField.Name {
        try XCTUnwrap(HTTPField.Name("Withdrawal-Authorization"))
    }

    private func withdrawKeys(_ transport: StubTransport) async throws -> [String?] {
        let header = try keyHeader()
        return await transport.sent.filter { $0.path == "/v1/me/withdrawals" }.map { $0.headerFields[header] }
    }

    private let authorization = "01890a5d-ac96-774b-bcce-b302099a8059"

    private var granted: StubTransport.Reply {
        .json(.created, #"{"authorization_id":"\#(authorization)","expires_at":"2026-10-21T12:05:00Z"}"#)
    }

    private func withdrawing(_ transport: StubTransport) -> Withdrawing {
        Withdrawing(source: WithdrawSource(api: api(transport)), hints: FakeHintStream(), inFlightRetryDelay: .zero)
    }

    private func api(_ transport: StubTransport) -> APIClient {
        APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
    }

    private func waitUntil(_ predicate: @escaping () async -> Bool) async -> Bool {
        for _ in 0..<500 {
            if await predicate() { return true }
            await Task.yield()
        }
        return await predicate()
    }
}
