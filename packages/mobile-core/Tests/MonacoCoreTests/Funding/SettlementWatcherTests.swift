import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class SettlementWatcherTests: XCTestCase {
    private let transferID = "01890a5d-ac96-774b-bcce-b302099a8057"
    private let withdrawalID = "01890a5d-ac96-774b-bcce-b302099a8060"
    private let cabalID = "01890a5d-ac96-774b-bcce-b302099a8059"

    private func fundTicket() -> SettlementTicket {
        SettlementTicket(kind: .fund, resourceID: transferID, cabalID: cabalID, cabalName: "Sunday Investors")
    }

    private func read(_ status: String) -> String {
        #"{"status":"\#(status)","amount_micros":"5000000","share_units":null,"fail_code":null}"#
    }

    private func watcher(
        _ transport: StubTransport, hints: FakeHintStream = FakeHintStream(), journal: PendingIntentJournal
    ) -> SettlementWatcher {
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        return SettlementWatcher(api: api, hints: hints, journal: journal)
    }

    private func waitUntil(_ predicate: @escaping () async -> Bool) async -> Bool {
        for _ in 0..<1000 {
            if await predicate() { return true }
            await Task.yield()
        }
        return await predicate()
    }

    func testAWatchedFundSettlesIntoANoticeAndLeavesTheJournal() async {
        let journal = PendingIntentJournal()
        let transport = StubTransport(scripted: [.json(.ok, read("submitted")), .json(.ok, read("settled"))])
        let hints = FakeHintStream()
        let watcher = watcher(transport, hints: hints, journal: journal)
        watcher.bind(accountID: "a")

        watcher.watch(fundTicket())
        XCTAssertEqual(journal.intents(accountID: "a").count, 1)
        _ = await waitUntil { await hints.subscriberCount == 2 }
        await hints.send(.changed(.cabal(cabalID), what: "activity_changed", id: "1"))

        let done = await waitUntil { watcher.notice != nil }
        XCTAssertTrue(done)
        guard case .fund(.settled(let transfer)) = watcher.notice?.outcome else {
            return XCTFail("\(String(describing: watcher.notice))")
        }
        XCTAssertEqual(transfer.amountMicros, 5_000_000)
        XCTAssertEqual(watcher.notice?.ticket.cabalName, "Sunday Investors")
        XCTAssertTrue(journal.intents(accountID: "a").isEmpty)
        XCTAssertEqual(watcher.watchedCount, 0)
    }

    func testSigningOutCancelsTheWatchersButKeepsTheIntent() async {
        let journal = PendingIntentJournal()
        let transport = StubTransport(scripted: [.json(.ok, read("submitted"))])
        let hints = FakeHintStream()
        let watcher = watcher(transport, hints: hints, journal: journal)
        watcher.bind(accountID: "a")
        watcher.watch(fundTicket())
        _ = await waitUntil { await hints.subscriberCount == 2 }
        XCTAssertEqual(watcher.watchedCount, 1)

        watcher.reset()

        XCTAssertEqual(watcher.watchedCount, 0)
        let unsubscribed = await waitUntil { await hints.subscriberCount == 0 }
        XCTAssertTrue(unsubscribed)
        let reads = await transport.sent.count
        await hints.send(.changed(.cabal(cabalID), what: "activity_changed", id: "1"))
        for _ in 0..<50 { await Task.yield() }
        let after = await transport.sent.count
        XCTAssertEqual(after, reads)
        XCTAssertNil(watcher.notice)
        XCTAssertEqual(journal.intents(accountID: "a").count, 1)
    }

    func testTheWatcherSurvivesTheScreenAndResumesAfterRelaunchOnlyForItsAccount() async {
        let journal = PendingIntentJournal()
        let first = watcher(StubTransport(.hang), journal: journal)
        first.bind(accountID: "a")
        first.watch(fundTicket())
        first.reset()

        let transport = StubTransport(scripted: [.json(.ok, read("settled"))])
        let other = watcher(transport, journal: journal)
        other.bind(accountID: "b")
        for _ in 0..<50 { await Task.yield() }
        XCTAssertEqual(other.watchedCount, 0)
        let none = await transport.sent.count
        XCTAssertEqual(none, 0)

        let relaunched = watcher(transport, journal: journal)
        relaunched.bind(accountID: "a")
        let done = await waitUntil { relaunched.notice != nil }

        XCTAssertTrue(done)
        guard case .fund(.settled) = relaunched.notice?.outcome else {
            return XCTFail("\(String(describing: relaunched.notice))")
        }
        XCTAssertTrue(journal.intents(accountID: "a").isEmpty)
    }

    func testAWithdrawalIsWatchedToConfirmation() async {
        let journal = PendingIntentJournal()
        let confirmed =
            #"{"withdrawal_id":"\#(withdrawalID)","status":"confirmed","amount_micros":"2000000","#
            + #""to_address":"wallet","tx_signature":"sig","fail_code":null,"#
            + #""created_at":"2026-10-03T15:00:00Z","completed_at":"2026-10-03T15:00:10Z"}"#
        let watcher = watcher(StubTransport(scripted: [.json(.ok, confirmed)]), journal: journal)
        watcher.bind(accountID: "a")

        watcher.watch(
            SettlementTicket(kind: .withdraw, resourceID: withdrawalID, fullBalanceLabel: "$2.00 (full balance)"))

        let done = await waitUntil { watcher.notice != nil }
        XCTAssertTrue(done)
        guard case .withdraw(.confirmed) = watcher.notice?.outcome else {
            return XCTFail("\(String(describing: watcher.notice))")
        }
        XCTAssertEqual(watcher.withdrawals[withdrawalID]?.isSettled, true)
        XCTAssertTrue(journal.intents(accountID: "a").isEmpty)
    }

    func testAFailedFundShownOnScreenMakesNoSecondNotice() async {
        let failed = #"{"status":"failed","amount_micros":"5000000","share_units":null,"fail_code":"fund_not_sent"}"#
        let watcher = watcher(StubTransport(scripted: [.json(.ok, failed)]), journal: PendingIntentJournal())
        watcher.bind(accountID: "a")
        watcher.present(transferID)

        watcher.watch(fundTicket())

        let settled = await waitUntil { watcher.fund[self.transferID]?.isSettled == true }
        XCTAssertTrue(settled)
        for _ in 0..<50 { await Task.yield() }
        XCTAssertNil(watcher.notice)
        XCTAssertEqual(watcher.watchedCount, 0)
    }
}
