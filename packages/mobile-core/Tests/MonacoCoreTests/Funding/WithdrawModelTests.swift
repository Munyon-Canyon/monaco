import Foundation
import HTTPTypes
import MonacoAPI
import MonacoCore
import MonacoTestClock
import MonacoTestSupport
import XCTest

@MainActor
final class WithdrawModelTests: XCTestCase {
    private let solana = "9xQeWvG816bUx9EPjHmaT23yvVMvM9fQj4a8PHF4H6P"
    private let ownSolana = "7EcDhSYGxXyscszYEp35KHN8vvw3svAuLKTzXwCFLtV"
    private let evm = "0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed"
    private let withdrawalID = "01890a5d-ac96-774b-bcce-b302099a8057"
    let previewPath = "/v1/me/withdrawals/preview"

    func testTheEmptyStateReadsTheFeeAndTheServersMaxNotTheBalance() async {
        let rig = rig([previewPath: [empty]])
        rig.model.load()
        await flush(rig)

        XCTAssertEqual(rig.model.state, .empty)
        XCTAssertEqual(rig.model.line, "Fee $0.50 · USDC on Solana")
        XCTAssertEqual(rig.model.maxMicros, 499_500_000)
        XCTAssertEqual(rig.model.quote?.availableMicros, 500_000_000)
        XCTAssertNil(rig.model.buttonTitle)
        let bodies = await previewBodies(rig.transport)
        XCTAssertEqual(bodies, [[:]])
    }

    func testATypedAmountWithAnAddressShowsWhatArrivesAndTheButton() async {
        let rig = rig([previewPath: [typed]])
        rig.model.setAddress(solana)
        rig.model.setAmountText("120")
        await flush(rig)

        XCTAssertEqual(rig.model.state, .typed)
        XCTAssertEqual(rig.model.line, "≈ 120.00 USDC · fee $0.50")
        XCTAssertEqual(rig.model.buttonTitle, "Withdraw $120")
        XCTAssertTrue(rig.model.canSubmit)
        let bodies = await previewBodies(rig.transport)
        XCTAssertEqual(bodies, [["amount_micros": "120000000", "to_address": solana]])
    }

    func testAnAmountOverTheMaxIsInvalidWithTheOverAmountAndNoButton() async {
        let rig = rig([previewPath: [over]])
        rig.model.setAddress(solana)
        rig.model.setAmountText("650")
        await flush(rig)

        XCTAssertEqual(rig.model.state, .invalid(maxMicros: 499_500_000, overMicros: 150_500_000))
        XCTAssertEqual(rig.model.line, "≈ 650.00 USDC · fee $0.50")
        XCTAssertEqual(rig.model.limitSlot, "Up to $499.50")
        XCTAssertNil(rig.model.buttonTitle)
        XCTAssertFalse(rig.model.canSubmit)
    }

    func testAWithinLimitAmountShowsTheServerReceiveEstimate() async {
        let rig = rig([previewPath: [quote(receive: "119500000", over: "0")]])
        rig.model.setAddress(solana)
        rig.model.setAmountText("120")
        await flush(rig)

        XCTAssertEqual(rig.model.state, .typed)
        XCTAssertEqual(rig.model.line, "≈ 119.50 USDC · fee $0.50")
    }

    func testAnAmountUnderTheMinimumIsBelowMinimum() async {
        let rig = rig([previewPath: [quote(belowMin: true)]])
        rig.model.setAddress(solana)
        rig.model.setAmountText("0.5")
        await flush(rig)

        XCTAssertEqual(rig.model.state, .belowMinimum(minMicros: 1_000_000))
        XCTAssertEqual(rig.model.line, "The minimum is $1")
        XCTAssertFalse(rig.model.canSubmit)
    }

    func testAnAmountWithoutAValidAddressNeedsOneAndDisablesTheButton() async {
        let rig = rig([previewPath: [typed]])
        rig.model.setAmountText("120")
        await flush(rig)

        XCTAssertEqual(rig.model.state, .needsAddress)
        XCTAssertEqual(rig.model.buttonTitle, "Add an address")
        XCTAssertFalse(rig.model.canSubmit)
        XCTAssertNil(rig.model.addressProblem)
    }

    func testAnUnknownFeeSaysItWillShowOnceYouType() async {
        let rig = rig([previewPath: [quote(fee: nil, mode: "included", max: "500000000", arrival: 120)]])
        rig.model.setNetwork(.base)
        await flush(rig)

        XCTAssertEqual(rig.model.line, "Fee shown once you type · USDC on Base")
        XCTAssertEqual(rig.model.maxMicros, 500_000_000)
    }

    func testAZeroFeeSaysNoFee() async {
        let rig = rig([previewPath: [quote(fee: "0", max: "500000000")]])
        rig.model.load()
        await flush(rig)

        XCTAssertEqual(rig.model.line, "No fee · USDC on Solana")
    }

    func testANetworkChangeRequotesOnThatNetworkAndFlagsASolanaAddressOnBase() async {
        let rig = rig([previewPath: [empty, quote(fee: "7500", mode: "included", max: "500000000", arrival: 120)]])
        rig.model.setAddress(solana)
        rig.model.load()
        await flush(rig)
        XCTAssertNil(rig.model.addressProblem)

        rig.model.setNetwork(.base)
        await flush(rig)

        XCTAssertNotNil(rig.model.addressProblem)
        XCTAssertNil(rig.model.validAddress)
        XCTAssertEqual(rig.model.quote?.arrivalSeconds, 120)
        let bodies = await previewBodies(rig.transport)
        XCTAssertEqual(bodies.count, 2)
        XCTAssertEqual(bodies.last?["network"], "base")
        XCTAssertEqual(bodies.last?["destination_kind"], "evm")
        XCTAssertNil(bodies.first?["network"])
    }

    func testYourOwnDepositAddressOnSolanaIsFlaggedWithoutTheWordWallet() async {
        let rig = rig([previewPath: []])
        rig.model.setAddress(ownSolana)

        let message = rig.model.addressProblem
        XCTAssertEqual(message, WithdrawCopy.ownDeposit)
        XCTAssertFalse(message?.localizedCaseInsensitiveContains("wallet") ?? true)
        XCTAssertNil(rig.model.validAddress)
    }

    func testAnEVMAddressOnBaseIsValid() async {
        let rig = rig([previewPath: []])
        rig.model.setNetwork(.base)
        rig.model.setAddress(evm)

        XCTAssertEqual(rig.model.validAddress, evm)
        XCTAssertNil(rig.model.addressProblem)
    }

    func testOnlyTheLaunchMoneyChainsAreOffered() {
        let model = rig([:]).model
        XCTAssertEqual(model.networks, [.solana, .base])
        for hidden in [WithdrawNetwork.arbitrum, .ethereum, .monad] {
            model.setNetwork(hidden)
            XCTAssertEqual(model.network, .solana)
        }
    }

    func testThreeEditsInOnePauseSendOneQuote() async {
        let rig = rig([previewPath: [typed]])
        rig.model.setAddress(solana)
        rig.model.setAmountText("1")
        rig.model.setAmountText("12")
        rig.model.setAmountText("120")
        await flush(rig)

        let bodies = await previewBodies(rig.transport)
        XCTAssertEqual(bodies, [["amount_micros": "120000000", "to_address": solana]])
    }

    func testAFailedQuoteRetriesOnceAfterTwoSecondsThenOffersATap() async {
        let rig = rig([previewPath: [.failure(URLError(.timedOut)), empty]])
        rig.model.load()
        _ = await rig.clock.state.until { $0.pending >= 1 }
        rig.clock.advance(by: .milliseconds(250))
        _ = await rig.clock.state.until { $0.requested.contains(.seconds(2)) }
        rig.clock.advance(by: .seconds(2))
        await waitUntil { !rig.model.isQuoting }

        XCTAssertFalse(rig.model.quoteFailed)
        XCTAssertEqual(rig.model.line, "Fee $0.50 · USDC on Solana")
    }

    func testTwoFailuresShowTheFeeErrorAndATapQuotesAgain() async {
        let rig = rig([previewPath: [.failure(URLError(.timedOut)), .failure(URLError(.timedOut)), empty]])
        rig.model.load()
        _ = await rig.clock.state.until { $0.pending >= 1 }
        rig.clock.advance(by: .milliseconds(250))
        _ = await rig.clock.state.until { $0.requested.contains(.seconds(2)) }
        rig.clock.advance(by: .seconds(2))
        await waitUntil { rig.model.quoteFailed }

        XCTAssertEqual(rig.model.line, "Couldn't get the fee. Try again.")
        rig.model.retryQuote()
        await flush(rig)
        XCTAssertFalse(rig.model.quoteFailed)
        XCTAssertEqual(rig.model.line, "Fee $0.50 · USDC on Solana")
    }

    func testMaxFillsTheServersMaxAmount() async {
        let rig = rig([previewPath: [empty, typed]])
        rig.model.load()
        await flush(rig)
        rig.model.useMax()

        XCTAssertEqual(rig.model.amountText, "499.5")
        XCTAssertEqual(rig.model.amountMicros, 499_500_000)
    }

    func testSettledOnlyOnceTheServerSaysSubmittedAndTheBalanceReloads() async {
        let rig = rig(
            [
                previewPath: [typed],
                "/v1/me/withdrawals/authorizations": [granted],
                "/v1/me/withdrawals": [accepted],
                "/v1/me/balance": [balance("379500000")],
            ], withBalance: true)
        rig.model.setAddress(solana)
        rig.model.setAmountText("120")
        await flush(rig)

        await rig.model.submit()

        XCTAssertEqual(
            rig.model.state,
            .settled(
                amountMicros: 120_000_000, to: solana, network: .solana, arrivalSeconds: 60,
                cashAfterMicros: 379_500_000))
        XCTAssertEqual(rig.model.line, "Sent just now · arrives on Solana in about a minute")
        XCTAssertEqual(rig.model.checkLine, "$120 to 9xQe…4H6P")
    }

    func testARefusedSubmitDoesNotSettle() async throws {
        let refusal = try StubTransport.Reply.problem(
            Components.Schemas.Problem(
                _type: .about_colon_blank, title: "Unprocessable", status: 422, code: .insufficientFunds,
                message: "Not enough in your account balance.", traceId: "4bf92f3577b34da6a3ce929d0e0e4736",
                retryable: false))
        let rig = rig(
            [
                previewPath: [typed], "/v1/me/withdrawals/authorizations": [granted], "/v1/me/withdrawals": [refusal],
            ])
        rig.model.setAddress(solana)
        rig.model.setAmountText("120")
        await flush(rig)

        await rig.model.submit()

        XCTAssertEqual(rig.model.state, .failed("Not enough in your account balance."))
    }

    func testTheIdempotencyKeyIsReusedAfterATransportError() async throws {
        let rig = rig(
            [
                previewPath: [typed],
                "/v1/me/withdrawals/authorizations": [granted, granted],
                "/v1/me/withdrawals": [.failure(URLError(.timedOut)), accepted],
            ])
        rig.model.setAddress(solana)
        rig.model.setAmountText("120")
        await flush(rig)

        await rig.model.submit()
        guard case .failed = rig.model.state else { return XCTFail("\(rig.model.state) is not failed") }
        XCTAssertTrue(rig.model.canSubmit)
        await rig.model.submit()

        guard case .settled = rig.model.state else { return XCTFail("\(rig.model.state) is not settled") }
        let key = try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
        let keys = await rig.transport.sent.filter { $0.path == "/v1/me/withdrawals" }.map { $0.headerFields[key] }
        XCTAssertEqual(keys, ["key-1", "key-1"])
    }
}

extension WithdrawModelTests {
    private func quote(
        fee: String? = "500000", mode: String = "on_top", max: String = "499500000", receive: String = "0",
        over: String? = nil, belowMin: Bool = false, arrival: Int = 60
    ) -> StubTransport.Reply {
        let feeField = fee.map { #""\#($0)""# } ?? "null"
        let overField = over.map { #""\#($0)""# } ?? "null"
        return .json(
            .ok,
            """
            {"available_micros":"500000000","fee_micros":\(feeField),"fee_known":\(fee != nil),"fee_mode":"\(mode)",\
            "max_amount_micros":"\(max)","receive_micros":"\(receive)","over_micros":\(overField),\
            "below_min":\(belowMin),"min_micros":"1000000","arrival_seconds":\(arrival),"expires_at":null}
            """)
    }

    private var empty: StubTransport.Reply { quote() }
    private var typed: StubTransport.Reply { quote(receive: "120000000", over: "0") }
    private var over: StubTransport.Reply { quote(over: "150500000") }

    private var granted: StubTransport.Reply {
        .json(
            .created,
            #"{"authorization_id":"01890a5d-ac96-774b-bcce-b302099a8059","expires_at":"2026-10-21T12:05:00Z"}"#)
    }

    private var accepted: StubTransport.Reply {
        .json(.accepted, #"{"withdrawal_id":"\#(withdrawalID)","status":"submitted","tx_signature":"signature-1"}"#)
    }

    private func balance(_ available: String) -> StubTransport.Reply {
        .json(
            .ok,
            #"{"available_micros":"\#(available)","on_chain_micros":"\#(available)","in_flight_micros":"0","#
                + #""deposit_address":"\#(ownSolana)","as_of":"2026-10-28T12:00:00Z"}"#)
    }

    private struct Rig {
        let model: WithdrawModel
        let clock: TestClock
        let transport: StubTransport
    }

    private func rig(
        _ routes: [String: [StubTransport.Reply]], withBalance: Bool = false
    ) -> Rig {
        let transport = StubTransport(routes: routes)
        let api = APIClient(
            serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        let hints = FakeHintStream()
        let clock = TestClock()
        let model = WithdrawModel(
            api: api, hints: hints, submission: IdempotentSubmission(makeKey: { "key-1" }),
            settlements: SettlementWatcher(api: api, hints: hints, journal: PendingIntentJournal()),
            ownDepositAddress: ownSolana, clock: clock,
            balance: withBalance ? BalanceSource(api: api, hints: hints) : nil)
        return Rig(model: model, clock: clock, transport: transport)
    }

    private func waitUntil(_ predicate: @escaping () -> Bool) async {
        for _ in 0..<2000 {
            if predicate() { return }
            await Task.yield()
        }
    }

    private func flush(_ rig: Rig) async {
        _ = await rig.clock.state.until { $0.pending >= 1 }
        rig.clock.advance(by: .milliseconds(250))
        await waitUntil { !rig.model.isQuoting }
    }

    private func previewBodies(_ transport: StubTransport) async -> [[String: String]] {
        let sent = await transport.sent
        let bodies = await transport.sentBodies
        return zip(sent, bodies).compactMap { request, body in
            guard request.path == previewPath, let body else { return nil }
            return try? JSONSerialization.jsonObject(with: body) as? [String: String]
        }
    }
}
