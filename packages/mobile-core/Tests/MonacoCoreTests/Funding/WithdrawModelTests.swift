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
    private var withdrawalPath: String { "/v1/me/withdrawals/\(withdrawalID)" }

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
        XCTAssertEqual(rig.model.line, "120.00 USDC · fee 0.50 USDC")
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
        XCTAssertEqual(rig.model.line, "650.00 USDC · fee 0.50 USDC")
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
        XCTAssertEqual(rig.model.line, "119.50 USDC · fee 0.50 USDC")
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
                withdrawalPath: [read("submitted")],
            ])
        rig.model.setAddress(solana)
        rig.model.setAmountText("120")
        await flush(rig)

        await rig.model.submit()
        guard case .failed = rig.model.state else { return XCTFail("\(rig.model.state) is not failed") }
        XCTAssertTrue(rig.model.canSubmit)
        await rig.model.submit()

        guard case .processing = rig.model.state else { return XCTFail("\(rig.model.state) is not processing") }
        let key = try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
        let keys = await rig.transport.sent.filter { $0.path == "/v1/me/withdrawals" }.map { $0.headerFields[key] }
        XCTAssertEqual(keys, ["key-1", "key-1"])
    }
}

extension WithdrawModelTests {
    func testAnAcceptedWithdrawalShowsProcessingNotSuccessUntilTheWatcherConfirms() async {
        let rig = rig(
            [
                previewPath: [typed],
                "/v1/me/withdrawals/authorizations": [granted],
                "/v1/me/withdrawals": [accepted],
                "/v1/me/balance": [balance("379500000")],
                withdrawalPath: [read("submitted")],
            ], withBalance: true)
        rig.model.setAddress(solana)
        rig.model.setAmountText("120")
        await flush(rig)

        await rig.model.submit()

        XCTAssertEqual(
            rig.model.state, .processing(amountMicros: 120_000_000, to: solana, network: .solana, arrivalSeconds: 60))
        XCTAssertEqual(rig.model.buttonTitle, "Withdrawal processing · Pending")
        XCTAssertEqual(rig.model.line, "Arrives on Solana in about a minute.")
        XCTAssertNil(rig.model.checkLine)
    }

    func testTheWatcherConfirmingTurnsProcessingIntoSentWithTheBalanceAfter() async {
        let rig = rig(
            [
                previewPath: [typed],
                "/v1/me/withdrawals/authorizations": [granted],
                "/v1/me/withdrawals": [accepted],
                "/v1/me/balance": [balance("379500000")],
                withdrawalPath: [read("confirmed")],
            ], withBalance: true)
        rig.model.setAddress(solana)
        rig.model.setAmountText("120")
        await flush(rig)

        await rig.model.submit()
        await waitUntil { if case .settled = rig.model.state { true } else { false } }

        XCTAssertEqual(
            rig.model.state,
            .settled(
                amountMicros: 120_000_000, to: solana, network: .solana, arrivalSeconds: 60,
                cashAfterMicros: 379_500_000))
        XCTAssertEqual(rig.model.line, "Sent just now · arrives on Solana in about a minute")
        XCTAssertEqual(rig.model.checkLine, "$120 to 9xQe…4H6P")
    }

    func testAWithdrawalThatFailsAfterTheAcceptKeepsNoSuccessAndCanBeSentAgain() async throws {
        let rig = rig(
            [
                previewPath: [typed],
                "/v1/me/withdrawals/authorizations": [granted, granted],
                "/v1/me/withdrawals": [accepted, accepted],
                withdrawalPath: [read("failed")],
            ])
        rig.model.setAddress(solana)
        rig.model.setAmountText("120")
        await flush(rig)

        await rig.model.submit()
        await waitUntil { if case .failed = rig.model.state { true } else { false } }

        XCTAssertEqual(rig.model.state, .failed(WithdrawProgress.didNotGoThrough))
        XCTAssertNil(rig.model.checkLine)
        XCTAssertTrue(rig.model.canSubmit)
        await rig.model.submit()
        let posts = await rig.transport.sent.filter { $0.path == "/v1/me/withdrawals" }.count
        XCTAssertEqual(posts, 2)
    }

    func testTheAmountAndAddressAreLockedWhileTheWithdrawalProcesses() async {
        let rig = rig(
            [
                previewPath: [typed],
                "/v1/me/withdrawals/authorizations": [granted],
                "/v1/me/withdrawals": [accepted],
                withdrawalPath: [read("submitted")],
            ])
        rig.model.setAddress(solana)
        rig.model.setAmountText("120")
        await flush(rig)

        await rig.model.submit()
        rig.model.setAmountText("500")
        rig.model.setAddress(evm)
        rig.model.setNetwork(.base)

        XCTAssertEqual(rig.model.amountText, "120")
        XCTAssertEqual(rig.model.address, solana)
        XCTAssertEqual(rig.model.network, .solana)
    }

    func testAFailureAfterTheScreenClosedStillReachesTheGlobalNotice() async {
        let rig = rig(
            [
                previewPath: [typed],
                "/v1/me/withdrawals/authorizations": [granted],
                "/v1/me/withdrawals": [accepted],
                withdrawalPath: [read("failed")],
            ])
        rig.model.setAddress(solana)
        rig.model.setAmountText("120")
        await flush(rig)
        rig.model.dismiss()

        await rig.model.submit()
        await waitUntil { rig.settlements.notice != nil }

        guard case .withdraw(.failed) = rig.settlements.notice?.outcome else {
            return XCTFail("\(String(describing: rig.settlements.notice))")
        }
    }

    func testAFailureWhileTheScreenIsOpenIsShownOnTheScreenNotAsAGlobalNotice() async {
        let rig = rig(
            [
                previewPath: [typed],
                "/v1/me/withdrawals/authorizations": [granted],
                "/v1/me/withdrawals": [accepted],
                withdrawalPath: [read("failed")],
            ])
        rig.model.setAddress(solana)
        rig.model.setAmountText("120")
        await flush(rig)

        await rig.model.submit()
        await waitUntil { if case .failed = rig.model.state { true } else { false } }
        await Task.yield()

        XCTAssertEqual(rig.model.state, .failed(WithdrawProgress.didNotGoThrough))
        XCTAssertNil(rig.settlements.notice)
    }

    func testABaseAmountWithNoAddressShowsNoEstimateNotZero() async {
        let rig = rig([previewPath: [quote(fee: "7500", mode: "included", max: "500000000", receive: "0", over: "0")]])
        rig.model.setNetwork(.base)
        rig.model.setAmountText("120")
        await flush(rig)

        XCTAssertEqual(rig.model.state, .needsAddress)
        XCTAssertNil(rig.model.quote?.receiveMicros)
        XCTAssertEqual(rig.model.line, "Fee $0.01 · USDC on Base")
        XCTAssertFalse(rig.model.line?.contains("0.00 USDC") ?? false)
    }

    func testABaseAmountWithAnAddressShowsTheEstimateWithATilde() async {
        let reply = quote(fee: "7500", mode: "included", max: "500000000", receive: "119992500", over: "0")
        let rig = rig([previewPath: [reply]])
        rig.model.setNetwork(.base)
        rig.model.setAddress(evm)
        rig.model.setAmountText("120")
        await flush(rig)

        XCTAssertEqual(rig.model.line, "≈ 119.99 USDC · fee 0.01 USDC")
    }

    func testASolanaAmountReadsExactlyInOneUnit() async {
        let rig = rig([previewPath: [typed]])
        rig.model.setAddress(solana)
        rig.model.setAmountText("120")
        await flush(rig)

        XCTAssertEqual(rig.model.line, "120.00 USDC · fee 0.50 USDC")
    }

    func testARefusedAddressKeepsWithdrawDisabledUntilTheAddressChanges() async throws {
        let refusal = try StubTransport.Reply.problem(
            Components.Schemas.Problem(
                _type: .about_colon_blank, title: "Unprocessable", status: 422, code: .invalidAddress,
                message: "Paste a wallet address.", traceId: "4bf92f3577b34da6a3ce929d0e0e4736", retryable: false))
        let rig = rig(
            [
                previewPath: [typed, typed], "/v1/me/withdrawals/authorizations": [granted],
                "/v1/me/withdrawals": [refusal],
            ])
        rig.model.setAddress(solana)
        rig.model.setAmountText("120")
        await flush(rig)
        XCTAssertTrue(rig.model.buttonIsEnabled)

        await rig.model.submit()

        XCTAssertEqual(rig.model.addressProblem, WithdrawCopy.invalidAddress)
        XCTAssertFalse(rig.model.canSubmit)
        XCTAssertFalse(rig.model.buttonIsEnabled)
        await rig.model.submit()
        let sent = await rig.transport.sent.filter { $0.path == "/v1/me/withdrawals" }
        XCTAssertEqual(sent.count, 1)

        rig.model.setAddress(solana + " ")
        XCTAssertNil(rig.model.addressProblem)
    }

    func testAnAddressTheQuoteRefusesNamesTheAddressNotTheFee() async throws {
        let refusal = try StubTransport.Reply.problem(
            Components.Schemas.Problem(
                _type: .about_colon_blank, title: "Unprocessable", status: 422, code: .invalidAddress,
                message: "Paste a wallet address.", traceId: "4bf92f3577b34da6a3ce929d0e0e4736", retryable: false))
        let rig = rig([previewPath: [refusal, empty]])
        rig.model.setAddress(solana)
        rig.model.setAmountText("120")
        await flush(rig)

        XCTAssertEqual(rig.model.addressProblem, WithdrawCopy.invalidAddress)
        XCTAssertFalse(rig.model.quoteFailed)
        XCTAssertNotEqual(rig.model.line, WithdrawCopy.feeFailed)
        XCTAssertFalse(rig.model.canSubmit)
        XCTAssertFalse(rig.model.buttonIsEnabled)
        let previews = await rig.transport.sent.filter { $0.path == previewPath }
        XCTAssertEqual(previews.count, 1)

        rig.model.setAddress(solana + " ")
        XCTAssertNil(rig.model.addressProblem)
    }

    func testADifferentWithdrawUnderAPendingKeyNamesThePendingOneAndPrefillsIt() async throws {
        let rig = rig(
            [
                previewPath: [typed, typed, typed],
                "/v1/me/withdrawals/authorizations": [granted],
                "/v1/me/withdrawals": [.failure(URLError(.timedOut))],
            ])
        rig.model.setAddress(solana)
        rig.model.setAmountText("120")
        await flush(rig)
        await rig.model.submit()
        rig.model.setAmountText("50")
        await flush(rig)

        await rig.model.submit()

        let pendingLine = WithdrawCopy.pendingBlocked(amountMicros: 120_000_000, address: solana)
        XCTAssertEqual(rig.model.state, .failed(pendingLine))
        XCTAssertEqual(rig.model.amountText, "120")
        XCTAssertEqual(rig.model.address, solana)
        XCTAssertEqual(rig.model.buttonTitle, "Check again")
        await flush(rig)
        XCTAssertTrue(rig.model.canSubmit)
        let sent = await rig.transport.sent.filter { $0.path == "/v1/me/withdrawals" }
        XCTAssertEqual(sent.count, 1)
    }

    func testWithdrawIsDisabledWhileANewQuoteLoads() async {
        let rig = rig([previewPath: [typed, typed]])
        rig.model.setAddress(solana)
        rig.model.setAmountText("120")
        await flush(rig)
        XCTAssertTrue(rig.model.buttonIsEnabled)

        rig.model.setAmountText("121")

        XCTAssertEqual(rig.model.state, .typed)
        XCTAssertFalse(rig.model.canSubmit)
        XCTAssertFalse(rig.model.buttonIsEnabled)
        await flush(rig)
        XCTAssertTrue(rig.model.buttonIsEnabled)
    }

    func testAFailedQuoteKeepsTheButtonLiveToRetry() async {
        let rig = rig([previewPath: [.failure(URLError(.timedOut)), .failure(URLError(.timedOut))]])
        rig.model.setAddress(solana)
        rig.model.setAmountText("120")
        _ = await rig.clock.state.until { $0.pending >= 1 }
        rig.clock.advance(by: .milliseconds(250))
        _ = await rig.clock.state.until { $0.requested.contains(.seconds(2)) }
        rig.clock.advance(by: .seconds(2))
        await waitUntil { rig.model.quoteFailed }

        XCTAssertTrue(rig.model.quoteFailed)
        XCTAssertTrue(rig.model.buttonIsEnabled)
    }

    func testAZeroXAddressOnSolanaOffersBaseAndSwitchingKeepsTheAddress() async {
        let rig = rig([previewPath: [empty]])
        rig.model.setAddress(evm)

        XCTAssertEqual(rig.model.suggestedNetwork, .base)
        XCTAssertEqual(rig.model.addressProblem, WithdrawCopy.baseAddressOnSolana)
        XCTAssertFalse(rig.model.addressProblem?.contains("\"0\"") ?? true)

        rig.model.switchToSuggestedNetwork()

        XCTAssertEqual(rig.model.network, .base)
        XCTAssertEqual(rig.model.address, evm)
        XCTAssertEqual(rig.model.validAddress, evm)
        XCTAssertNil(rig.model.suggestedNetwork)
        XCTAssertNil(rig.model.addressProblem)
    }

    func testASolanaAddressOnSolanaOffersNothing() {
        let rig = rig([previewPath: []])
        rig.model.setAddress(solana)
        XCTAssertNil(rig.model.suggestedNetwork)
    }

    func testABalanceHintRequotesSoMaxAndCashAreFresh() async {
        let rig = rig([previewPath: [empty, quote(max: "299500000")]])
        rig.model.load()
        await flush(rig)
        XCTAssertEqual(rig.model.maxMicros, 499_500_000)
        let observing = Task { await rig.model.observe() }
        await rig.hints.waitForSubscribers(1)

        await rig.hints.send(.changed(.user("u"), what: "balance_changed", id: "1"))
        await flush(rig)

        XCTAssertEqual(rig.model.maxMicros, 299_500_000)
        observing.cancel()
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

    private func read(_ status: String) -> StubTransport.Reply {
        .json(
            .ok,
            #"{"withdrawal_id":"\#(withdrawalID)","status":"\#(status)","amount_micros":"120000000","#
                + #""to_address":"\#(solana)","tx_signature":"signature-1","fail_code":null,"#
                + #""created_at":"2026-10-28T12:00:00Z","completed_at":null}"#)
    }

    private func balance(_ available: String) -> StubTransport.Reply {
        .json(
            .ok,
            #"{"available_micros":"\#(available)","on_chain_micros":"\#(available)","in_flight_micros":"0","#
                + #""deposit_address":"\#(ownSolana)","as_of":"2026-10-28T12:00:00Z"}"#)
    }

    private struct Rig {
        let model: WithdrawModel
        let settlements: SettlementWatcher
        let clock: TestClock
        let transport: StubTransport
        let hints: FakeHintStream
    }

    private func rig(
        _ routes: [String: [StubTransport.Reply]], withBalance: Bool = false
    ) -> Rig {
        let transport = StubTransport(routes: routes)
        let api = APIClient(
            serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        let hints = FakeHintStream()
        let clock = TestClock()
        let settlements = SettlementWatcher(api: api, hints: hints, journal: PendingIntentJournal())
        let model = WithdrawModel(
            api: api, hints: hints, submission: IdempotentSubmission(makeKey: { "key-1" }),
            settlements: settlements,
            ownDepositAddress: ownSolana, clock: clock,
            balance: withBalance ? BalanceSource(api: api, hints: hints) : nil)
        return Rig(model: model, settlements: settlements, clock: clock, transport: transport, hints: hints)
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
