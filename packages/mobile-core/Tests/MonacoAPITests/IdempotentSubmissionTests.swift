import Foundation
#if canImport(FoundationNetworking)
import FoundationNetworking
#endif
import MonacoAPI
import MonacoTestSupport
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

final class SubmitTests: XCTestCase {
    func testARetryAfterATransportErrorSendsTheSameKey() async throws {
        let transport = StubTransport(scripted: [.failure(URLError(.networkConnectionLost)), Fixtures.ping])
        let client = Fixtures.client(transport)
        let submission = IdempotentSubmission()

        await assertThrows(.transport(URLError(.networkConnectionLost))) { _ = try await client.ping(submission) }
        _ = try await client.ping(submission)

        let keys = await transport.idempotencyKeys
        XCTAssertEqual(keys.count, 2)
        XCTAssertNotNil(keys[0])
        XCTAssertEqual(keys[0], keys[1])
    }

    func testAChangedPayloadSendsANewKey() async throws {
        let transport = StubTransport(scripted: [.failure(URLError(.timedOut)), Fixtures.ping])
        let client = Fixtures.client(transport)
        let submission = IdempotentSubmission()

        _ = try? await client.ping(submission, note: "hi")
        _ = try await client.ping(submission, note: "hello")

        let keys = await transport.idempotencyKeys
        XCTAssertNotEqual(keys[0], keys[1])
    }

    func testA201ClearsTheKey() async throws {
        let transport = StubTransport(Fixtures.ping)
        let client = Fixtures.client(transport)
        let submission = IdempotentSubmission()

        let ping = try await client.ping(submission)
        XCTAssertFalse(submission.hasPendingKey)
        _ = try await client.ping(submission)

        XCTAssertEqual(ping.note, "hi")
        let keys = await transport.idempotencyKeys
        XCTAssertNotEqual(keys[0], keys[1])
    }

    func testAnInFlight409KeepsTheKeyAndSurfacesInFlight() async throws {
        let transport = StubTransport(scripted: [Fixtures.problem(409, "idempotency_in_flight"), Fixtures.ping])
        let client = Fixtures.client(transport)
        let submission = IdempotentSubmission()

        await assertThrows(.inFlight) { _ = try await client.ping(submission) }
        XCTAssertTrue(submission.hasPendingKey)
        _ = try await client.ping(submission)

        let keys = await transport.idempotencyKeys
        XCTAssertEqual(keys[0], keys[1])
    }

    func testOnlyAFinal4xxClearsTheKey() async throws {
        let answers: [(StubTransport.Reply, final: Bool)] = [
            (Fixtures.problem(422, "invalid_input"), true),
            (Fixtures.problem(409, "version_conflict"), true),
            (Fixtures.problem(403, "account_deleted"), true),
            (Fixtures.problem(429, "rate_limited"), false),
            (Fixtures.problem(503, "upstream_unavailable"), false),
            (.response(status: .ok, contentType: "application/json", body: Data("{".utf8)), false),
        ]
        for (reply, final) in answers {
            let submission = IdempotentSubmission()

            _ = try? await Fixtures.client(StubTransport(reply)).ping(submission)

            XCTAssertEqual(submission.hasPendingKey, !final, "\(reply)")
        }
    }
}
