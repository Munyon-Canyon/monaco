import Foundation
import HTTPTypes
import MonacoAPI
import MonacoCore
import MonacoTestClock
import MonacoTestSupport
import XCTest

@MainActor
final class FundCabalModelTests: XCTestCase {
    private let cabalID = "01890a5d-ac96-774b-bcce-b302099a8059"
    private let transferID = "01890a5d-ac96-774b-bcce-b302099a8057"
    private let cabalName = "AC Capital"

    private struct Staged: Codable {
        let micros: Int64
    }

    private struct Rig {
        let model: FundCabalModel
        let watcher: SettlementWatcher
        let transport: StubTransport
        let clock: TestClock
        let hints: FakeHintStream
        let submission: IdempotentSubmission
    }

    private func preview(
        available: String = "500000000", minimum: String = "1000000", now: Int = 3500, after: Int? = nil,
        over: String = "0", paused: Bool = false, first: String? = "2026-10-03T15:00:00Z"
    ) -> String {
        let after = after.map(String.init) ?? "null"
        let first = first.map { #""\#($0)""# } ?? "null"
        return #"{"available_micros":"\#(available)","min_micros":"\#(minimum)","slice_bps_now":\#(now),"#
            + #""slice_bps_after":\#(after),"over_micros":"\#(over)","paused":\#(paused),"#
            + #""first_funded_at":\#(first)}"#
    }

    private var accepted: String {
        #"{"transfer_id":"\#(transferID)","status":"submitted"}"#
    }

    private func transfer(_ status: String, sliceBps: Int? = nil) -> String {
        let slice = sliceBps.map(String.init) ?? "null"
        let units = status == "settled" ? #""250000000""# : "null"
        let code = status == "failed" ? #""fund_not_sent""# : "null"
        return #"{"status":"\#(status)","amount_micros":"250000000","share_units":\#(units),"#
            + #""slice_bps":\#(slice),"fail_code":\#(code)}"#
    }

    private func problem(_ status: Int, _ code: Components.Schemas.ErrorCode) -> Components.Schemas.Problem {
        Components.Schemas.Problem(
            _type: .about_colon_blank, title: "Refused", status: status, code: code, message: "Server says no.",
            traceId: "4bf92f3577b34da6a3ce929d0e0e4736", retryable: false)
    }

    private func rig(
        _ transport: StubTransport, journal: PendingIntentJournal = PendingIntentJournal()
    ) -> Rig {
        let clock = TestClock()
        let hints = FakeHintStream()
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        let watcher = SettlementWatcher(api: api, hints: hints, journal: journal, schedule: .standard(on: clock))
        watcher.bind(accountID: "a")
        let submission = IdempotentSubmission()
        let model = FundCabalModel(
            api: api, hints: hints, cabalID: cabalID, cabalName: cabalName, submission: submission,
            settlements: watcher, clock: clock)
        return Rig(
            model: model, watcher: watcher, transport: transport, clock: clock, hints: hints, submission: submission)
    }

    private func type(_ text: String, into rig: Rig) async {
        let before = rig.clock.state.current.requested.count
        rig.model.amountText = text
        rig.model.amountChanged()
        _ = await rig.clock.state.until { $0.requested.count > before }
        rig.clock.advance(by: .milliseconds(250))
    }

    private func settle(_ rig: Rig, _ expected: FundCabalState) async {
        _ = await waitUntil { rig.model.state == expected }
        XCTAssertEqual(rig.model.state, expected)
    }

    private func waitUntil(_ predicate: @escaping () async -> Bool) async -> Bool {
        for _ in 0..<2000 {
            if await predicate() { return true }
            await Task.yield()
        }
        return await predicate()
    }

    private func keyHeader() throws -> HTTPField.Name {
        try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
    }

    private func typedRig(
        _ rest: [StubTransport.Reply], now: Int = 3500, first: String? = "2026-10-03T15:00:00Z"
    ) async -> Rig {
        let rig = rig(
            StubTransport(
                scripted: [
                    .json(.ok, preview(now: now, first: first)),
                    .json(.ok, preview(now: now, after: now + 100, first: first)),
                ] + rest))
        await rig.model.load()
        await type("250", into: rig)
        await settle(rig, .typed(sliceNow: Int32(now), sliceAfter: Int32(now + 100)))
        return rig
    }

    func testLoadShowsTheEmptyStateAndTheMaxIsTheAvailableCash() async {
        let rig = rig(StubTransport(scripted: [.json(.ok, preview())]))

        await rig.model.load()

        XCTAssertEqual(rig.model.state, .empty)
        XCTAssertEqual(rig.model.maxMicros, 500_000_000)
        XCTAssertEqual(rig.model.line, "From your cash · your share 35%")
        XCTAssertEqual(rig.model.slotHint, "Enter an amount")
        XCTAssertNil(rig.model.buttonTitle)
        let sent = await rig.transport.sent
        XCTAssertEqual(sent.first?.path, "/v1/cabals/\(cabalID)/fund/preview")
    }

    func testATypedAmountShowsTheShareBeforeAndAfterAndTheButton() async {
        let rig = await typedRig([])

        XCTAssertEqual(rig.model.line, "From your cash · your share 35% → 36%")
        XCTAssertEqual(rig.model.buttonTitle, "Fund $250")
        XCTAssertNil(rig.model.limitLine)
        let sent = await rig.transport.sent
        XCTAssertEqual(sent.last?.path, "/v1/cabals/\(cabalID)/fund/preview?amount_micros=250000000")
    }

    func testStalePricesKeepTheLineAtFromYourCashWithoutAShare() async throws {
        let rig = rig(
            StubTransport(scripted: [.json(.ok, preview()), try .problem(problem(503, .pricesStale))]))
        await rig.model.load()

        await type("250", into: rig)
        _ = await waitUntil { rig.model.shareUnavailable }

        XCTAssertEqual(rig.model.line, "From your cash")
        XCTAssertEqual(rig.model.maxMicros, 500_000_000)
    }

    func testStalePricesOnTheFirstLoadShowFromYourCashNotAFailure() async throws {
        let rig = rig(StubTransport(scripted: [try .problem(problem(503, .pricesStale))]))

        await rig.model.load()

        XCTAssertEqual(rig.model.state, .empty)
        XCTAssertEqual(rig.model.line, "From your cash")
        XCTAssertTrue(rig.model.shareUnavailable)
    }

    func testTheShareComesBackWhenThePricesDo() async throws {
        let rig = rig(
            StubTransport(
                scripted: [
                    try .problem(problem(503, .pricesStale)), .json(.ok, preview()),
                ]))
        await rig.model.load()

        await rig.model.load()

        XCTAssertFalse(rig.model.shareUnavailable)
        XCTAssertEqual(rig.model.line, "From your cash · your share 35%")
    }

    func testAnOtherFailureOnTheFirstLoadStillShowsItsMessage() async throws {
        let rig = rig(StubTransport(scripted: [try .problem(problem(422, .insufficientFunds))]))

        await rig.model.load()

        XCTAssertEqual(rig.model.state, .failed("Server says no."))
        XCTAssertFalse(rig.model.shareUnavailable)
    }

    func testAnAmountOverTheCashIsInvalidWithoutAButton() async {
        let rig = rig(
            StubTransport(scripted: [.json(.ok, preview()), .json(.ok, preview(after: 3658, over: "150000000"))]))
        await rig.model.load()

        await type("650", into: rig)

        await settle(rig, .invalid(availableMicros: 500_000_000, overMicros: 150_000_000))
        XCTAssertEqual(rig.model.line, "From your cash · your share 35%")
        XCTAssertEqual(rig.model.limitLine, "$500.00 available · $150.00 over")
        XCTAssertEqual(rig.model.slotHint, "Up to $500.00")
        XCTAssertNil(rig.model.buttonTitle)
        XCTAssertFalse(rig.model.canSubmit)
    }

    func testAnAmountUnderTheMinimumNamesIt() async {
        let rig = rig(StubTransport(scripted: [.json(.ok, preview()), .json(.ok, preview())]))
        await rig.model.load()

        await type("0.5", into: rig)

        await settle(rig, .belowMinimum(minMicros: 1_000_000))
        XCTAssertEqual(rig.model.line, "Minimum $1")
        XCTAssertNil(rig.model.buttonTitle)
    }

    func testAPausedCabalSaysSoAndOffersNoButton() async {
        let rig = rig(StubTransport(scripted: [.json(.ok, preview(paused: true)), .json(.ok, preview(paused: true))]))
        await rig.model.load()
        XCTAssertEqual(rig.model.state, .paused)

        await type("250", into: rig)

        await settle(rig, .paused)
        XCTAssertEqual(rig.model.line, "AC Capital is paused. Funding opens when it resumes.")
        XCTAssertNil(rig.model.buttonTitle)
    }

    func testClearingTheAmountGoesBackToEmptyWithoutARequest() async {
        let rig = await typedRig([])

        rig.model.amountText = ""
        rig.model.amountChanged()

        XCTAssertEqual(rig.model.state, .empty)
        let count = await rig.transport.sent.count
        XCTAssertEqual(count, 2)
    }

    func testTheDebounceSendsOnePreviewPerPause() async {
        let rig = rig(StubTransport(scripted: [.json(.ok, preview()), .json(.ok, preview(after: 3600))]))
        await rig.model.load()

        rig.model.amountText = "2"
        rig.model.amountChanged()
        rig.model.amountText = "25"
        rig.model.amountChanged()
        await type("250", into: rig)
        await settle(rig, .typed(sliceNow: 3500, sliceAfter: 3600))

        let sent = await rig.transport.sent.map { $0.path ?? "" }
        XCTAssertEqual(
            sent.filter { $0.contains("amount_micros") },
            [
                "/v1/cabals/\(cabalID)/fund/preview?amount_micros=250000000"
            ])
    }
}

extension FundCabalModelTests {
    func testSubmitStagesThePendingAmountBeforeTheRequestAndFollowsTheTransfer() async {
        let rig = await typedRig([.gate, .json(.ok, transfer("submitted"))])

        let submitting = Task { await rig.model.submit() }
        await rig.transport.waitForRequests(3)

        XCTAssertEqual(rig.submission.pending(Staged.self)?.micros, 250_000_000)
        XCTAssertEqual(rig.model.state, .submitting)
        XCTAssertEqual(rig.model.line, "Funding…")
        await rig.transport.releaseGate(.json(.accepted, accepted))
        await submitting.value
        XCTAssertEqual(rig.model.watchedTransferID, transferID)
    }

    func testSettledOnlyAfterTheWatcherReadsSettledWithTheTransfersShare() async {
        let rig = await typedRig([
            .json(.accepted, accepted), .gate, .json(.ok, preview(available: "250000000", now: 3600)),
        ])

        await rig.model.submit()
        _ = await waitUntil { await rig.transport.sent.count == 4 }

        XCTAssertEqual(rig.model.state, .submitting)
        XCTAssertFalse(rig.model.line.contains("your share is now"))
        await rig.transport.releaseGate(.json(.ok, transfer("settled", sliceBps: 3658)))
        await settle(
            rig, .settled(amountMicros: 250_000_000, sliceNow: 3658, cashAfterMicros: 250_000_000))
        XCTAssertEqual(rig.model.line, "Added just now · your share is now 37%")
        XCTAssertEqual(rig.model.checkLabel, "$250 to AC Capital")
        XCTAssertNil(rig.model.buttonTitle)
    }

    func testTheCashAfterIsRefreshedByLoadOnceSettled() async {
        let rig = await typedRig([
            .json(.accepted, accepted), .json(.ok, transfer("settled", sliceBps: 3600)),
            .json(.ok, preview(available: "247500000", now: 3600)),
        ])
        await rig.model.submit()
        await settle(rig, .settled(amountMicros: 250_000_000, sliceNow: 3600, cashAfterMicros: 250_000_000))

        await rig.model.load()

        await settle(rig, .settled(amountMicros: 250_000_000, sliceNow: 3600, cashAfterMicros: 247_500_000))
    }

    func testAnOnScreenFailureShowsInTheModelAndPostsNoNotice() async {
        let rig = await typedRig([.json(.accepted, accepted), .json(.ok, transfer("failed"))])

        await rig.model.submit()

        await settle(rig, .failed(FundProgress.didNotGoThrough))
        for _ in 0..<50 { await Task.yield() }
        XCTAssertNil(rig.watcher.notice)
    }

    func testAFailureAfterDismissReachesTheWatchersNotice() async {
        let rig = await typedRig([.json(.accepted, accepted), .gate])
        await rig.model.submit()
        _ = await waitUntil { await rig.transport.sent.count == 4 }

        rig.model.dismiss()
        rig.model.dismiss()
        await rig.transport.releaseGate(.json(.ok, transfer("failed")))

        _ = await waitUntil { rig.watcher.notice != nil }
        guard case .fund(.failed) = rig.watcher.notice?.outcome else {
            return XCTFail("\(String(describing: rig.watcher.notice))")
        }
        XCTAssertEqual(rig.watcher.notice?.ticket.resourceID, transferID)
    }

    func testAFailureAfterClosingBeforeTheAcceptReachesTheWatchersNotice() async {
        let rig = await typedRig([.gate, .json(.ok, transfer("failed"))])
        let sending = Task { await rig.model.submit() }
        _ = await waitUntil { await rig.transport.sent.count == 3 }

        rig.model.dismiss()
        await rig.transport.releaseGate(.json(.accepted, accepted))
        await sending.value

        _ = await waitUntil { rig.watcher.notice != nil }
        guard case .fund(.failed) = rig.watcher.notice?.outcome else {
            return XCTFail("\(String(describing: rig.watcher.notice))")
        }
        XCTAssertEqual(rig.watcher.notice?.ticket.resourceID, transferID)
    }

    func testARelaunchedWatcherResumesTheJournaledTicket() async {
        let journal = PendingIntentJournal()
        let first = rig(
            StubTransport(
                scripted: [
                    .json(.ok, preview()), .json(.ok, preview(after: 3600)), .json(.accepted, accepted), .hang,
                ]),
            journal: journal)
        await first.model.load()
        await type("250", into: first)
        await settle(first, .typed(sliceNow: 3500, sliceAfter: 3600))
        await first.model.submit()
        first.watcher.reset()

        let relaunched = rig(
            StubTransport(scripted: [.json(.ok, transfer("settled", sliceBps: 3600))]), journal: journal)

        _ = await waitUntil { relaunched.watcher.notice != nil }
        guard case .fund(.settled(let settled)) = relaunched.watcher.notice?.outcome else {
            return XCTFail("\(String(describing: relaunched.watcher.notice))")
        }
        XCTAssertEqual(settled.sliceBps, 3600)
    }

    func testADroppedResponseLeavesUnconfirmedAndTheRetryReusesTheKey() async throws {
        let rig = await typedRig([.failure(URLError(.timedOut)), .json(.accepted, accepted), .hang])

        await rig.model.submit()

        XCTAssertEqual(rig.model.state, .unconfirmed(MoneyFlowCopy.unconfirmed.summary))
        XCTAssertEqual(rig.model.amountText, "250")
        XCTAssertTrue(rig.model.canSubmit)
        await rig.model.submit()
        XCTAssertEqual(rig.model.watchedTransferID, transferID)
        let header = try keyHeader()
        let keys = await rig.transport.sent.filter { $0.path?.hasSuffix("/fund") == true }.map {
            $0.headerFields[header]
        }
        XCTAssertEqual(keys.count, 2)
        XCTAssertEqual(keys[0], keys[1])
    }

    func testAChangedAmountUnderAPendingKeyNamesThePendingAmountAndPrefillsIt() async {
        let rig = await typedRig([.failure(URLError(.timedOut)), .json(.ok, preview(after: 3700))])
        await rig.model.submit()

        await type("300", into: rig)
        await settle(rig, .typed(sliceNow: 3500, sliceAfter: 3700))
        await rig.model.submit()

        XCTAssertEqual(rig.model.state, .unconfirmed(FundCabalCopy.pendingBlocked(micros: 250_000_000)))
        XCTAssertEqual(
            rig.model.line,
            "Your $250 fund may still be going through. Check it again before funding a different amount.")
        XCTAssertEqual(rig.model.amountText, "250")
        XCTAssertEqual(rig.model.buttonTitle, "Check again")
        XCTAssertTrue(rig.model.canSubmit)
        let posts = await rig.transport.sent.filter { $0.path?.hasSuffix("/fund") == true }
        XCTAssertEqual(posts.count, 1)
    }

    func testFundCannotBeSubmittedForAnAmountThePreviewHasNotSeen() async {
        let rig = await typedRig([.json(.ok, preview(after: 3700))])
        XCTAssertTrue(rig.model.canSubmit)

        let before = rig.clock.state.current.requested.count
        rig.model.amountText = "300"
        rig.model.amountChanged()

        XCTAssertFalse(rig.model.canSubmit)
        XCTAssertNil(rig.model.buttonTitle)
        await rig.model.submit()
        let early = await rig.transport.sent.filter { $0.path?.hasSuffix("/fund") == true }
        XCTAssertTrue(early.isEmpty)
        _ = await rig.clock.state.until { $0.requested.count > before }
        rig.clock.advance(by: .milliseconds(250))
        _ = await waitUntil { rig.model.canSubmit }
        XCTAssertEqual(rig.model.buttonTitle, "Fund $300")
    }

    func testRetryingAFailedFundSendsANewTransferUnderANewKey() async throws {
        let second = "01890a5d-ac96-774b-bcce-b302099a8058"
        let rig = await typedRig([
            .json(.accepted, accepted), .json(.ok, transfer("failed")),
            .json(.accepted, #"{"transfer_id":"\#(second)","status":"submitted"}"#), .hang,
        ])
        await rig.model.submit()
        await settle(rig, .failed(FundProgress.didNotGoThrough))
        XCTAssertTrue(rig.model.canSubmit)

        await rig.model.submit()

        XCTAssertEqual(rig.model.watchedTransferID, second)
        XCTAssertEqual(rig.model.state, .submitting)
        let header = try keyHeader()
        let keys = await rig.transport.sent.filter { $0.path?.hasSuffix("/fund") == true }.map {
            $0.headerFields[header]
        }
        XCTAssertEqual(keys.count, 2)
        XCTAssertNotEqual(keys[0], keys[1])
    }

    func testEditingTheAmountAfterAFailedFundPreviewsAgain() async {
        let rig = await typedRig([
            .json(.accepted, accepted), .json(.ok, transfer("failed")), .json(.ok, preview(after: 3550)),
        ])
        await rig.model.submit()
        await settle(rig, .failed(FundProgress.didNotGoThrough))

        await type("100", into: rig)

        await settle(rig, .typed(sliceNow: 3500, sliceAfter: 3550))
        XCTAssertNil(rig.model.watchedTransferID)
        XCTAssertTrue(rig.model.canSubmit)
    }

    func testARefusalShowsTheFailureCopyAndKeepsTheAmount() async throws {
        let rig = await typedRig([try .problem(problem(422, .insufficientFunds))])

        await rig.model.submit()

        XCTAssertEqual(rig.model.state, .failed(FundFailure.notEnoughBalance))
        XCTAssertEqual(rig.model.amountText, "250")
        XCTAssertNil(rig.model.watchedTransferID)
        XCTAssertTrue(rig.model.canSubmit)
    }

    func testFirstFundFollowsTheShareHeldBeforeTheFundNotFirstFundedAt() async {
        let never = await typedRig(
            [.json(.accepted, accepted), .json(.ok, transfer("settled", sliceBps: 100))], now: 0, first: nil)
        XCTAssertFalse(never.model.firstFund)
        await never.model.submit()
        await settle(never, .settled(amountMicros: 250_000_000, sliceNow: 100, cashAfterMicros: 250_000_000))
        XCTAssertTrue(never.model.firstFund)

        let cashedOut = await typedRig(
            [.json(.accepted, accepted), .json(.ok, transfer("settled", sliceBps: 100))], now: 0)
        await cashedOut.model.submit()
        await settle(cashedOut, .settled(amountMicros: 250_000_000, sliceNow: 100, cashAfterMicros: 250_000_000))
        XCTAssertTrue(cashedOut.model.firstFund)

        let before = await typedRig([.json(.accepted, accepted), .json(.ok, transfer("settled", sliceBps: 3600))])
        await before.model.submit()
        await settle(before, .settled(amountMicros: 250_000_000, sliceNow: 3600, cashAfterMicros: 250_000_000))
        XCTAssertFalse(before.model.firstFund)
    }

    func testABalanceHintReloadsThePreviewButNotWhileSubmitting() async {
        let rig = rig(StubTransport(scripted: [.json(.ok, preview()), .json(.ok, preview(available: "400000000"))]))
        await rig.model.load()
        let observing = Task { await rig.model.observe() }
        await rig.hints.waitForSubscribers(1)

        await rig.hints.send(.changed(.user("u"), what: "balance_changed", id: "1"))

        _ = await waitUntil { rig.model.maxMicros == 400_000_000 }
        XCTAssertEqual(rig.model.maxMicros, 400_000_000)
        observing.cancel()
    }

    func testAnObservedReloadWhileSubmittingSendsNothing() async {
        let rig = await typedRig([.gate])
        let observing = Task { await rig.model.observe() }
        await rig.hints.waitForSubscribers(1)
        let submitting = Task { await rig.model.submit() }
        await rig.transport.waitForRequests(3)

        await rig.hints.send(.resync)
        for _ in 0..<100 { await Task.yield() }

        let count = await rig.transport.sent.count
        XCTAssertEqual(count, 3)
        await rig.transport.releaseGate(.failure(URLError(.timedOut)))
        await submitting.value
        observing.cancel()
    }
}
