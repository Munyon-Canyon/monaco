import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import Synchronization
import XCTest

final class CardDepositStateTests: XCTestCase {
    private let id = "01890a5d-ac96-774b-bcce-b302099a8057"
    private let other = "01890a5d-ac96-774b-bcce-b302099a8058"

    func testStartingCreatesFromEveryRestingState() {
        for state: CardDepositState in [
            .idle, .browsing(sessionID: id), .processing(sessionID: id), .done(sessionID: id),
            .failed(sessionID: nil, message: "x"),
        ] {
            XCTAssertEqual(state.applying(.start), .creating, "\(state)")
        }
        XCTAssertEqual(CardDepositState.creating.applying(.start), .creating)
    }

    func testCreatingEndsInBrowsingOrFailed() {
        XCTAssertEqual(CardDepositState.creating.applying(.created(sessionID: id)), .browsing(sessionID: id))
        XCTAssertEqual(
            CardDepositState.creating.applying(.createFailed(message: "You're offline. Try again.")),
            .failed(sessionID: nil, message: "You're offline. Try again."))
        XCTAssertEqual(CardDepositState.idle.applying(.created(sessionID: id)), .idle)
        XCTAssertEqual(CardDepositState.idle.applying(.createFailed(message: "x")), .idle)
    }

    func testTheRedirectStartsProcessing() {
        XCTAssertEqual(
            CardDepositState.browsing(sessionID: id).applying(.redirected(sessionID: id)), .processing(sessionID: id))
        XCTAssertEqual(CardDepositState.idle.applying(.redirected(sessionID: id)), .processing(sessionID: id))
        XCTAssertEqual(
            CardDepositState.processing(sessionID: id).applying(.redirected(sessionID: id)), .processing(sessionID: id))
    }

    func testConfirmedOrSubmittedIsProcessing() {
        for status: OnrampStatus in [.confirmed, .submitted] {
            XCTAssertEqual(
                CardDepositState.browsing(sessionID: id).applying(.status(sessionID: id, status)),
                .processing(sessionID: id))
            XCTAssertEqual(
                CardDepositState.processing(sessionID: id).applying(.status(sessionID: id, status)),
                .processing(sessionID: id))
        }
    }

    func testCancelledFailedOrExpiredFails() {
        for status: OnrampStatus in [.cancelled, .failed, .expired] {
            for state: CardDepositState in [.browsing(sessionID: id), .processing(sessionID: id)] {
                XCTAssertEqual(
                    state.applying(.status(sessionID: id, status)),
                    .failed(sessionID: id, message: "Card purchase didn't go through."))
            }
        }
    }

    func testAnOpenSessionChangesNothing() {
        for status: OnrampStatus in [.created, .opened] {
            XCTAssertEqual(
                CardDepositState.browsing(sessionID: id).applying(.status(sessionID: id, status)),
                .browsing(sessionID: id))
        }
    }

    func testAnotherSessionsStatusChangesNothing() {
        XCTAssertEqual(
            CardDepositState.processing(sessionID: id).applying(.status(sessionID: other, .failed)),
            .processing(sessionID: id))
        XCTAssertEqual(CardDepositState.idle.applying(.status(sessionID: id, .confirmed)), .idle)
    }

    func testADepositEndsProcessing() {
        XCTAssertEqual(CardDepositState.processing(sessionID: id).applying(.deposited), .done(sessionID: id))
        XCTAssertEqual(CardDepositState.idle.applying(.deposited), .idle)
        XCTAssertEqual(CardDepositState.creating.applying(.deposited), .creating)
    }

    func testADepositHintBeforeTheRedirectStaysDone() {
        let landed = CardDepositState.browsing(sessionID: id).applying(.deposited)
        XCTAssertEqual(landed, .done(sessionID: id))
        XCTAssertEqual(landed.applying(.redirected(sessionID: id)), .done(sessionID: id))
    }

    func testAFailureHintBeforeTheRedirectStaysFailed() {
        let failed = CardDepositState.browsing(sessionID: id).applying(.status(sessionID: id, .cancelled))
        XCTAssertEqual(failed.applying(.redirected(sessionID: id)), failed)
    }
}

@MainActor
final class CardDepositTests: XCTestCase {
    private let id = "01890a5d-ac96-774b-bcce-b302099a8057"
    private let userID = "01890a5d-ac96-774b-bcce-b302099a8058"
    private let clock = WallClock()

    func testStartPostsTheShortfallAndOpensThePage() async throws {
        let transport = StubTransport(.json(.created, created))
        let deposit = CardDeposit(source: OnrampSource(api: api(transport)), hints: FakeHintStream(), now: clock.now)

        await deposit.start(suggestedMicros: 25_000_000, cabalID: userID)

        XCTAssertEqual(deposit.state, .browsing(sessionID: id))
        XCTAssertEqual(deposit.page?.url.absoluteString, "https://monacolabs.xyz/fund?s=tok")
        let sent = await transport.sent
        let request = try XCTUnwrap(sent.first)
        XCTAssertEqual(request.path, "/v1/onramp/sessions")
        let header = try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
        XCTAssertNotNil(request.headerFields[header])
        let bodies = await transport.sentBodies
        let body = try XCTUnwrap(bodies.first.flatMap { $0 })
        let json = try XCTUnwrap(JSONSerialization.jsonObject(with: body) as? [String: String])
        XCTAssertEqual(json, ["suggested_amount_micros": "25000000", "cabal_id": userID])
    }

    func testStartWithoutACabalSendsAnEmptyBody() async throws {
        let transport = StubTransport(.json(.created, created))
        let deposit = CardDeposit(source: OnrampSource(api: api(transport)), hints: FakeHintStream(), now: clock.now)

        await deposit.start(suggestedMicros: nil, cabalID: nil)

        let bodies = await transport.sentBodies
        let body = try XCTUnwrap(bodies.first.flatMap { $0 })
        let json = try XCTUnwrap(JSONSerialization.jsonObject(with: body) as? [String: String])
        XCTAssertEqual(json, [:])
    }

    func testAFailedStartToastsAndOpensNothing() async {
        let transport = StubTransport(.failure(URLError(.notConnectedToInternet)))
        let deposit = CardDeposit(source: OnrampSource(api: api(transport)), hints: FakeHintStream(), now: clock.now)

        await deposit.start(suggestedMicros: nil, cabalID: nil)

        XCTAssertNil(deposit.page)
        XCTAssertFalse(deposit.isCreating)
        XCTAssertEqual(deposit.message, "You're offline. Try again.")
        XCTAssertEqual(deposit.messageTick, 1)
    }

    func testTheRedirectClosesThePageAndReadsTheSession() async {
        let transport = StubTransport(scripted: [.json(.created, created), .json(.ok, session("confirmed"))])
        let deposit = CardDeposit(source: OnrampSource(api: api(transport)), hints: FakeHintStream(), now: clock.now)
        await deposit.start(suggestedMicros: nil, cabalID: nil)

        await deposit.redirected(sessionID: id)

        XCTAssertNil(deposit.page)
        XCTAssertTrue(deposit.isProcessing)
        let paths = await transport.sent.map(\.path)
        XCTAssertEqual(paths, ["/v1/onramp/sessions", "/v1/onramp/sessions/\(id)"])
    }

    func testACancelledRedirectToasts() async {
        let transport = StubTransport(scripted: [.json(.created, created), .json(.ok, session("cancelled"))])
        let deposit = CardDeposit(source: OnrampSource(api: api(transport)), hints: FakeHintStream(), now: clock.now)
        await deposit.start(suggestedMicros: nil, cabalID: nil)

        await deposit.redirected(sessionID: id)

        XCTAssertFalse(deposit.isProcessing)
        XCTAssertEqual(deposit.message, "Card purchase didn't go through.")
    }

    func testClosingTheBrowserReadsTheSessionOnceOnForeground() async {
        let transport = StubTransport(scripted: [.json(.created, created), .json(.ok, session("opened"))])
        let deposit = CardDeposit(source: OnrampSource(api: api(transport)), hints: FakeHintStream(), now: clock.now)
        await deposit.start(suggestedMicros: nil, cabalID: nil)

        deposit.browserClosed()
        await deposit.foregrounded()
        await deposit.foregrounded()

        XCTAssertEqual(deposit.state, .browsing(sessionID: id))
        XCTAssertNil(deposit.message)
        let count = await transport.sent.count
        XCTAssertEqual(count, 2)
    }

    func testAnOnrampHintReadsTheLiveSession() async {
        let transport = StubTransport(scripted: [.json(.created, created), .json(.ok, session("submitted"))])
        let hints = FakeHintStream()
        let deposit = CardDeposit(source: OnrampSource(api: api(transport)), hints: hints, now: clock.now)
        await deposit.start(suggestedMicros: nil, cabalID: nil)
        let observer = Task { await deposit.observe() }
        addTeardownBlock { observer.cancel() }
        _ = await waitUntil { await hints.subscriberCount == 1 }

        await hints.send(.changed(.user(userID), what: "onramp_changed", id: "1"))

        let processing = await waitUntil { deposit.isProcessing }
        XCTAssertTrue(processing)
        XCTAssertNil(deposit.page)
    }

    func testADepositEndsProcessing() async {
        let transport = StubTransport(scripted: [.json(.created, created), .json(.ok, session("confirmed"))])
        let deposit = CardDeposit(source: OnrampSource(api: api(transport)), hints: FakeHintStream(), now: clock.now)
        await deposit.start(suggestedMicros: nil, cabalID: nil)
        await deposit.redirected(sessionID: id)

        deposit.balanceChanged(nil)
        XCTAssertTrue(deposit.isProcessing)
        deposit.balanceChanged(.deposited(25_000_000))

        XCTAssertEqual(deposit.state, .done(sessionID: id))
    }

    func testADepositRowNewerThanTheRedirectEndsProcessing() async {
        let transport = StubTransport(scripted: processingScript(txns: [fundRow(at: 90), depositRow(at: 60)]))
        let deposit = CardDeposit(source: OnrampSource(api: api(transport)), hints: FakeHintStream(), now: clock.now)
        await deposit.start(suggestedMicros: nil, cabalID: nil)
        await deposit.redirected(sessionID: id)
        XCTAssertTrue(deposit.isProcessing)

        await deposit.foregrounded()

        XCTAssertEqual(deposit.state, .done(sessionID: id))
        let paths = await transport.sent.map(\.path)
        XCTAssertEqual(paths.last, "/v1/me/txns?limit=5")
    }

    func testADepositRowOlderThanTheRedirectLeavesProcessing() async {
        let transport = StubTransport(scripted: processingScript(txns: [fundRow(at: 90), depositRow(at: -60)]))
        let deposit = CardDeposit(source: OnrampSource(api: api(transport)), hints: FakeHintStream(), now: clock.now)
        await deposit.start(suggestedMicros: nil, cabalID: nil)
        await deposit.redirected(sessionID: id)

        await deposit.foregrounded()

        XCTAssertEqual(deposit.state, .processing(sessionID: id))
    }

    func testOnlyASettledDepositEndsProcessing() async {
        let pending = depositRow(at: 60, status: "pending")
        let transport = StubTransport(scripted: processingScript(txns: [pending]))
        let deposit = CardDeposit(source: OnrampSource(api: api(transport)), hints: FakeHintStream(), now: clock.now)
        await deposit.start(suggestedMicros: nil, cabalID: nil)
        await deposit.redirected(sessionID: id)

        await deposit.foregrounded()

        XCTAssertEqual(deposit.state, .processing(sessionID: id))
    }

    func testProcessingEndsAnHourAfterTheRedirect() async {
        let replies: [StubTransport.Reply] = [
            .json(.created, created), .json(.ok, session("confirmed")), .json(.ok, session("confirmed")),
        ]
        let transport = StubTransport(scripted: replies)
        let deposit = CardDeposit(
            source: OnrampSource(api: api(transport)), hints: FakeHintStream(), now: clock.now)
        await deposit.start(suggestedMicros: nil, cabalID: nil)
        await deposit.redirected(sessionID: id)

        clock.advance(by: 3600)
        XCTAssertTrue(deposit.isProcessing)
        clock.advance(by: 1)
        XCTAssertFalse(deposit.isProcessing)
        await deposit.foregrounded()

        XCTAssertEqual(deposit.state, .idle)
        let afterTimeout = await transport.sent.count
        XCTAssertEqual(afterTimeout, 2)
        await deposit.redirected(sessionID: id)
        XCTAssertTrue(deposit.isProcessing)
    }

    func testObservingChecksForADepositThatAlreadyLanded() async {
        let transport = StubTransport(scripted: processingScript(txns: [fundRow(at: 90), depositRow(at: 60)]))
        let deposit = CardDeposit(source: OnrampSource(api: api(transport)), hints: FakeHintStream(), now: clock.now)
        await deposit.start(suggestedMicros: nil, cabalID: nil)
        await deposit.redirected(sessionID: id)
        let observer = Task { await deposit.observe() }
        addTeardownBlock { observer.cancel() }

        let done = await waitUntil { [id] in deposit.state == .done(sessionID: id) }

        XCTAssertTrue(done)
    }

    func testABalanceHintChecksForADepositRow() async {
        let landed = [StubTransport.Reply.json(.ok, txnsPage([depositRow(at: 60)]))]
        let transport = StubTransport(scripted: processingScript(txns: []) + landed)
        let hints = FakeHintStream()
        let deposit = CardDeposit(source: OnrampSource(api: api(transport)), hints: hints, now: clock.now)
        await deposit.start(suggestedMicros: nil, cabalID: nil)
        await deposit.redirected(sessionID: id)
        let observer = Task { await deposit.observe() }
        addTeardownBlock { observer.cancel() }
        _ = await waitUntil { await hints.subscriberCount == 2 }
        XCTAssertTrue(deposit.isProcessing)

        await hints.send(.changed(.user(userID), what: BalanceSource.refreshingHint, id: "1"))

        let done = await waitUntil { [id] in deposit.state == .done(sessionID: id) }
        XCTAssertTrue(done)
    }

    func testTheSamplesDecode() {
        XCTAssertEqual(Components.Schemas.OnrampSession.sample.status, .confirmed)
        XCTAssertEqual(Components.Schemas.OnrampSessionCreated.sample.sessionId, id)
    }

    private var created: String {
        #"{"session_id":"\#(id)","url":"https://monacolabs.xyz/fund?s=tok","expires_at":"2025-10-04T12:10:00Z"}"#
    }

    private func session(_ status: String) -> String {
        #"{"session_id":"\#(id)","status":"\#(status)","suggested_amount_micros":null,"#
            + #""created_at":"2025-10-04T12:00:00Z","completed_at":null}"#
    }

    private func processingScript(txns: [String]) -> [StubTransport.Reply] {
        [.json(.created, created), .json(.ok, session("confirmed")), .json(.ok, txnsPage(txns))]
    }

    private func txn(_ kind: String, status: String, at seconds: TimeInterval) -> String {
        let stamp = ISO8601DateFormatter().string(from: Date().addingTimeInterval(seconds))
        return #"{"id":"\#(userID)","kind":"\#(kind)","status":"\#(status)","usdc_micros":"25000000","#
            + #""cabal":null,"tx_signature":"sig","created_at":"\#(stamp)"}"#
    }

    private func depositRow(at seconds: TimeInterval, status: String = "settled") -> String {
        txn("deposit", status: status, at: seconds)
    }

    private func fundRow(at seconds: TimeInterval) -> String {
        txn("fund", status: "settled", at: seconds)
    }

    private func txnsPage(_ items: [String]) -> String {
        #"{"items":[\#(items.joined(separator: ","))],"next_cursor":null}"#
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

private final class WallClock: Sendable {
    private let current = Mutex(Date())

    var now: @Sendable () -> Date { { [self] in current.withLock { $0 } } }

    func advance(by seconds: TimeInterval) {
        current.withLock { $0 += seconds }
    }
}
