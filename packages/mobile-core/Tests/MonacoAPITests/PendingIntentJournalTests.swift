import Foundation
import MonacoAPI
import Synchronization
import XCTest

final class PendingIntentJournalTests: XCTestCase {
    private let fingerprint = Data(#"fundCabal {"amount":1}"#.utf8)
    private let file = FileManager.default.temporaryDirectory.appendingPathComponent(
        "\(UUID().uuidString)/journal.json")
    private let clock = Clock()

    private final class Clock: Sendable {
        let now = Mutex(Date())
    }

    override func tearDown() {
        try? FileManager.default.removeItem(at: file.deletingLastPathComponent())
    }

    private func journal() -> PendingIntentJournal {
        PendingIntentJournal(fileURL: file, now: { [clock] in clock.now.withLock { $0 } })
    }

    func testARecordSurvivesANewJournalOnTheSameFile() {
        journal().save(
            PendingIntent(accountID: "a", slot: "fund:1", key: "key-1", fingerprint: fingerprint))

        let restored = journal().intent(accountID: "a", slot: "fund:1")

        XCTAssertEqual(restored?.key, "key-1")
        XCTAssertEqual(restored?.fingerprint, fingerprint)
    }

    func testAccountsNeverSeeEachOthersRecords() {
        let shared = journal()
        shared.save(PendingIntent(accountID: "a", slot: "withdraw", key: "key-a"))
        shared.save(PendingIntent(accountID: "b", slot: "withdraw", key: "key-b"))

        XCTAssertEqual(shared.intent(accountID: "a", slot: "withdraw")?.key, "key-a")
        XCTAssertEqual(shared.intent(accountID: "b", slot: "withdraw")?.key, "key-b")
        XCTAssertEqual(shared.intents(accountID: "a").count, 1)
        XCTAssertNil(shared.intent(accountID: "c", slot: "withdraw"))
        shared.remove(accountID: "a", slot: "withdraw")
        XCTAssertEqual(shared.intent(accountID: "b", slot: "withdraw")?.key, "key-b")
    }

    func testARecordOlderThanTheServersKeyRetentionIsDropped() {
        let shared = journal()
        shared.save(PendingIntent(accountID: "a", slot: "withdraw", key: "key-a"))

        clock.now.withLock { $0.addTimeInterval(PendingIntentJournal.retention - 60) }
        XCTAssertNotNil(shared.intent(accountID: "a", slot: "withdraw"))
        clock.now.withLock { $0.addTimeInterval(120) }
        XCTAssertNil(shared.intent(accountID: "a", slot: "withdraw"))
        XCTAssertTrue(journal().intents(accountID: "a").isEmpty)
    }

    func testAnUnreadableFileStartsEmpty() throws {
        try FileManager.default.createDirectory(
            at: file.deletingLastPathComponent(), withIntermediateDirectories: true)
        try Data("not json".utf8).write(to: file)

        XCTAssertTrue(journal().intents(accountID: "a").isEmpty)
    }

    func testASubmissionOnTheJournalKeepsItsKeyUntilAFinalAnswer() {
        let shared = journal()
        let first = IdempotentSubmission(journal: shared, accountID: "a", slot: "fund:1") { "key-1" }
        XCTAssertEqual(first.key(fingerprint: fingerprint), "key-1")
        first.record(final: false, forKey: "key-1")

        let reopened = IdempotentSubmission(journal: journal(), accountID: "a", slot: "fund:1") { "key-2" }
        XCTAssertTrue(reopened.hasPendingKey)
        XCTAssertEqual(reopened.key(fingerprint: fingerprint), "key-1")

        reopened.record(final: true, forKey: "key-1")
        XCTAssertNil(journal().intent(accountID: "a", slot: "fund:1"))
        XCTAssertEqual(reopened.key(fingerprint: fingerprint), "key-2")
    }

    func testASubmissionForAnotherAccountStartsWithoutAKey() {
        let shared = journal()
        let first = IdempotentSubmission(journal: shared, accountID: "a", slot: "fund:1") { "key-1" }
        _ = first.key(fingerprint: fingerprint)

        let other = IdempotentSubmission(journal: shared, accountID: "b", slot: "fund:1") { "key-b" }

        XCTAssertFalse(other.hasPendingKey)
        XCTAssertEqual(other.key(fingerprint: fingerprint), "key-b")
    }
}
