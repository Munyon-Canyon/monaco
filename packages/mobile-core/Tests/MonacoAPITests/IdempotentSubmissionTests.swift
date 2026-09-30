import Foundation
import MonacoAPI
import XCTest

final class IdempotentSubmissionTests: XCTestCase {
    private let fund = Data(#"fundGroup {"amount":1}"#.utf8)
    private let biggerFund = Data(#"fundGroup {"amount":2}"#.utf8)

    func testTheSameFingerprintKeepsItsKeyUntilAFinalAnswer() {
        let submission = countingSubmission()

        XCTAssertEqual(submission.key(fingerprint: fund), "key-1")
        submission.record(final: false, forKey: "key-1")
        XCTAssertEqual(submission.key(fingerprint: fund), "key-1")

        submission.record(final: true, forKey: "key-1")
        XCTAssertEqual(submission.key(fingerprint: fund), "key-2")
    }

    func testAChangedFingerprintGetsANewKey() {
        let submission = countingSubmission()

        XCTAssertEqual(submission.key(fingerprint: fund), "key-1")
        XCTAssertEqual(submission.key(fingerprint: biggerFund), "key-2")
    }

    func testALateAnswerForASupersededKeyKeepsTheCurrentKey() {
        let submission = countingSubmission()

        XCTAssertEqual(submission.key(fingerprint: fund), "key-1")
        XCTAssertEqual(submission.key(fingerprint: biggerFund), "key-2")
        submission.record(final: true, forKey: "key-1")

        XCTAssertEqual(submission.key(fingerprint: biggerFund), "key-2")
        XCTAssertTrue(submission.hasPendingKey)
    }

    func testHasPendingKeyFollowsTheKeyFromMintToFinalAnswer() {
        let submission = IdempotentSubmission { "key-1" }
        XCTAssertFalse(submission.hasPendingKey)

        let key = submission.key(fingerprint: fund)
        XCTAssertTrue(submission.hasPendingKey)

        submission.record(final: false, forKey: key)
        XCTAssertTrue(submission.hasPendingKey)

        submission.record(final: true, forKey: key)
        XCTAssertFalse(submission.hasPendingKey)
    }

    func testTheDefaultKeyIsALowercaseUUID() {
        let key = IdempotentSubmission().key(fingerprint: fund)

        XCTAssertNotNil(UUID(uuidString: key))
        XCTAssertEqual(key, key.lowercased())
    }

    private func countingSubmission() -> IdempotentSubmission {
        let counter = KeyCounter()
        return IdempotentSubmission { counter.next() }
    }
}

final class KeyCounter: @unchecked Sendable {
    private let lock = NSLock()
    private var count = 0

    func next() -> String {
        lock.lock()
        defer { lock.unlock() }
        count += 1
        return "key-\(count)"
    }
}
