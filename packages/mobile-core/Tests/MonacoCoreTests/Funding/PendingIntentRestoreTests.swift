import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class PendingIntentRestoreTests: XCTestCase {
    private let cabalID = "01890a5d-ac96-774b-bcce-b302099a8059"
    private let transferID = "01890a5d-ac96-774b-bcce-b302099a8057"
    private let withdrawalID = "01890a5d-ac96-774b-bcce-b302099a8060"
    private let alice = "user-alice"
    private let bob = "user-bob"
    private let directory = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)

    override func tearDown() async throws {
        try? FileManager.default.removeItem(at: directory)
    }

    private var journalFile: URL { directory.appendingPathComponent("pending-intents.json") }

    private func api(_ server: ReplayingServer) -> APIClient {
        APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-secret-1"), transport: server)
    }

    private func funding(_ server: ReplayingServer, _ journal: PendingIntentJournal, account: String) -> Funding {
        Funding(
            cabalID: cabalID, source: FundSource(api: api(server)), hints: FakeHintStream(), inFlightRetryDelay: .zero,
            submission: IdempotentSubmission(
                journal: journal, accountID: account, slot: Funding.slot(cabalID: cabalID)))
    }

    private func withdrawing(_ server: ReplayingServer, _ journal: PendingIntentJournal, account: String)
        -> Withdrawing
    {
        Withdrawing(
            source: WithdrawSource(api: api(server)), hints: FakeHintStream(), inFlightRetryDelay: .zero,
            submission: IdempotentSubmission(journal: journal, accountID: account, slot: Withdrawing.slot))
    }

    private var fundAccepted: String { #"{"transfer_id":"\#(transferID)","status":"submitted"}"# }
    private var withdrawAccepted: String {
        #"{"withdrawal_id":"\#(withdrawalID)","status":"submitted","tx_signature":"signature-1"}"#
    }
    private let address = "7EcDhSYGxXyscszYEp35KHN8vvw3svAuLKTzXwCFLtV"

    func testFundTimeoutThenDismissAndReopenReusesTheKeyAndMovesMoneyOnce() async {
        let server = ReplayingServer(accepted: fundAccepted)
        let journal = PendingIntentJournal()

        let first = await funding(server, journal, account: alice).submit(micros: 5_000_000)
        guard case .unconfirmed = first else { return XCTFail("\(first)") }
        let reopened = funding(server, journal, account: alice)
        let second = await reopened.submit(micros: 5_000_000)

        guard case .accepted(let transfer) = second else { return XCTFail("\(second)") }
        XCTAssertEqual(transfer.transferID, transferID)
        let executions = await server.executions
        let keys = await server.postKeys
        XCTAssertEqual(executions, 1)
        XCTAssertEqual(keys.count, 2)
        XCTAssertEqual(keys[0], keys[1])
        XCTAssertTrue(journal.intents(accountID: alice).isEmpty)
    }

    func testFundTimeoutThenAppRelaunchReusesTheKeyAndMovesMoneyOnce() async {
        let server = ReplayingServer(accepted: fundAccepted)
        _ = await funding(server, PendingIntentJournal(fileURL: journalFile), account: alice)
            .submit(micros: 5_000_000)

        let relaunched = PendingIntentJournal(fileURL: journalFile)
        let second = await funding(server, relaunched, account: alice).submit(micros: 5_000_000)

        guard case .accepted = second else { return XCTFail("\(second)") }
        let executions = await server.executions
        let keys = await server.postKeys
        XCTAssertEqual(executions, 1)
        XCTAssertEqual(Set(keys).count, 1)
    }

    func testAChangedFundAmountAfterRelaunchSendsNothing() async {
        let server = ReplayingServer(accepted: fundAccepted)
        _ = await funding(server, PendingIntentJournal(fileURL: journalFile), account: alice)
            .submit(micros: 5_000_000)

        let relaunched = funding(server, PendingIntentJournal(fileURL: journalFile), account: alice)
        let changed = await relaunched.submit(micros: 6_000_000)

        XCTAssertEqual(changed, .unconfirmed(retryMessage: MoneyFlowCopy.unconfirmed.summary))
        let sent = await server.postKeys.count
        XCTAssertEqual(sent, 1)
    }

    func testAnotherAccountNeitherSeesNorResendsTheFirstAccountsFund() async {
        let server = ReplayingServer(accepted: fundAccepted, lostResponses: 1)
        let journal = PendingIntentJournal(fileURL: journalFile)
        _ = await funding(server, journal, account: alice).submit(micros: 5_000_000)

        XCTAssertTrue(journal.intents(accountID: bob).isEmpty)
        let other = funding(server, PendingIntentJournal(fileURL: journalFile), account: bob)
        let result = await other.submit(micros: 6_000_000)

        guard case .accepted = result else { return XCTFail("\(result)") }
        let keys = await server.postKeys
        XCTAssertEqual(keys.count, 2)
        XCTAssertNotEqual(keys[0], keys[1])
        XCTAssertEqual(journal.intents(accountID: alice).count, 1)
    }

    func testWithdrawTimeoutThenReopenAndRelaunchReusesTheKeyAndSendsOnce() async {
        let server = ReplayingServer(accepted: withdrawAccepted)
        let journal = PendingIntentJournal(fileURL: journalFile)
        let first = await withdrawing(server, journal, account: alice).submit(micros: 2_000_000, toAddress: address)
        guard case .unconfirmed = first else { return XCTFail("\(first)") }

        let changed = await withdrawing(server, journal, account: alice)
            .submit(micros: 3_000_000, toAddress: address)
        XCTAssertEqual(changed, .unconfirmed(retryMessage: MoneyFlowCopy.unconfirmed.summary))
        let elsewhere = await withdrawing(server, journal, account: alice)
            .submit(micros: 2_000_000, toAddress: "8" + address.dropFirst())
        XCTAssertEqual(elsewhere, .unconfirmed(retryMessage: MoneyFlowCopy.unconfirmed.summary))

        let relaunched = PendingIntentJournal(fileURL: journalFile)
        let second = await withdrawing(server, relaunched, account: alice)
            .submit(micros: 2_000_000, toAddress: address)

        guard case .accepted(let withdrawal) = second else { return XCTFail("\(second)") }
        XCTAssertEqual(withdrawal.withdrawalID, withdrawalID)
        let executions = await server.executions
        let keys = await server.postKeys
        XCTAssertEqual(executions, 1)
        XCTAssertEqual(Set(keys).count, 1)
    }

    func testAnotherAccountNeitherSeesNorResendsTheFirstAccountsWithdrawal() async {
        let server = ReplayingServer(accepted: withdrawAccepted)
        let journal = PendingIntentJournal(fileURL: journalFile)
        _ = await withdrawing(server, journal, account: alice).submit(micros: 2_000_000, toAddress: address)

        let result = await withdrawing(server, journal, account: bob).submit(micros: 2_000_000, toAddress: address)

        guard case .accepted = result else { return XCTFail("\(result)") }
        let keys = await server.postKeys
        XCTAssertNotEqual(keys[0], keys[1])
    }

    func testCashOutTimeoutThenRelaunchResendsTheOriginalRequestUnderTheOriginalKey() async throws {
        let server = try cashOutServer()
        let first = try await loadedCashOut(server, PendingIntentJournal(fileURL: journalFile), account: alice)
        let attempt = await first.submit(enteredMicros: 200_100_000)
        XCTAssertEqual(attempt, .refused(MoneyFlowCopy.unconfirmed.summary))

        let relaunched = try await loadedCashOut(server, PendingIntentJournal(fileURL: journalFile), account: alice)
        XCTAssertTrue(relaunched.maySubmit(enteredMicros: 200_100_000))
        let changed = await relaunched.submit(enteredMicros: 100_000_000)
        XCTAssertEqual(changed, .refused(MoneyFlowCopy.unconfirmed.summary))
        let second = await relaunched.submit(enteredMicros: 200_100_000)

        guard case .started = second else { return XCTFail("\(String(describing: second))") }
        let executions = await server.executions
        let keys = await server.postKeys
        XCTAssertEqual(executions, 1)
        XCTAssertEqual(Set(keys).count, 1)
    }

    func testAnotherAccountDoesNotInheritAPendingCashOut() async throws {
        let server = try cashOutServer()
        let journal = PendingIntentJournal(fileURL: journalFile)
        let first = try await loadedCashOut(server, journal, account: alice)
        _ = await first.submit(enteredMicros: 200_100_000)

        let other = try await loadedCashOut(server, journal, account: bob)
        XCTAssertTrue(other.maySubmit(enteredMicros: 100_000_000))
        let result = await other.submit(enteredMicros: 100_000_000)

        guard case .started = result else { return XCTFail("\(String(describing: result))") }
        let keys = await server.postKeys
        XCTAssertNotEqual(keys[0], keys[1])
    }

    func testTheJournalFileHoldsNoAuthToken() async throws {
        let server = ReplayingServer(accepted: withdrawAccepted)
        _ = await withdrawing(server, PendingIntentJournal(fileURL: journalFile), account: alice)
            .submit(micros: 2_000_000, toAddress: address)

        let text = String(decoding: try Data(contentsOf: journalFile), as: UTF8.self)

        XCTAssertFalse(text.isEmpty)
        XCTAssertFalse(text.contains("token-secret-1"))
        XCTAssertFalse(text.lowercased().contains("authorization"))
        XCTAssertFalse(text.lowercased().contains("bearer"))
        let record = try XCTUnwrap(JSONSerialization.jsonObject(with: Data(text.utf8)) as? [String: [[String: Any]]])
        let keys = try XCTUnwrap(record["intents"]?.first).keys
        XCTAssertEqual(Set(keys), ["accountID", "slot", "key", "fingerprint", "context", "createdAt"])
    }

    private func cashOutServer() throws -> ReplayingServer {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        let job = try encoder.encode(Components.Schemas.CashOutJob.sample(status: .started))
        let preview = try encoder.encode(Components.Schemas.CashOutPreview.sample)
        return ReplayingServer(
            accepted: String(decoding: job, as: UTF8.self),
            reads: ["/v1/cabals/\(cabalID)/cashouts/preview": String(decoding: preview, as: UTF8.self)])
    }

    private func loadedCashOut(_ server: ReplayingServer, _ journal: PendingIntentJournal, account: String)
        async throws -> CashOutModel
    {
        let model = CashOutModel(
            cabalID: cabalID, api: api(server), hints: FakeHintStream(),
            submission: IdempotentSubmission(
                journal: journal, accountID: account, slot: CashOutModel.slot(cabalID: cabalID)))
        await model.load()
        return model
    }
}
