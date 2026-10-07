import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

private final class MemoryStore: KeyValueStoring {
    var values: [String: Any] = [:]

    func data(forKey key: String) -> Data? { values[key] as? Data }
    func bool(forKey key: String) -> Bool { values[key] as? Bool ?? false }
    func set(_ value: Any?, forKey key: String) { values[key] = value }
    func removeObject(forKey key: String) { values[key] = nil }
}

@MainActor
final class ReferralAttacherTests: XCTestCase {
    private static let created =
        #"{"referrer":{"user_id":"01890a5d-ac96-774b-bcce-b302099a8058","display_name":"Kai","#
        + #""photo_url":null,"handle":"kaicenat"}}"#
    private let now = Date(timeIntervalSince1970: 1_800_000_000)
    private let userID = "01890a5d-ac96-774b-bcce-b302099a8059"

    func testACreatedAttachSendsTheCodeAndSourceClearsAndToastsTheReferrer() async throws {
        let (attacher, transport, store) = try make([.json(.created, Self.created)], pending: .clipboard)

        let result = await attacher.attachPending(userID: userID)

        XCTAssertEqual(
            result, ReferralAttachResult(outcome: .attached, toast: "You joined from @kaicenat's invite."))
        XCTAssertFalse(attacher.hasPending)
        XCTAssertTrue(attacher.hasAttached(userID: userID))
        XCTAssertFalse(attacher.hasAttached(userID: "someone-else"))
        let requests = await transport.sent
        XCTAssertEqual(requests.map(\.path), ["/v1/me/referral"])
        XCTAssertNotNil(Self.key(requests[0]))
        let bodies = await transport.sentBodies
        let body = try JSONSerialization.jsonObject(with: try XCTUnwrap(bodies[0])) as? [String: String]
        XCTAssertEqual(body, ["code": "k7m4qx2p", "source": "clipboard"])
        XCTAssertNil(store.data(forKey: PendingReferralStore.key))
    }

    func testARefusalToastsTheServerMessageAndClears() async throws {
        let (attacher, _, _) = try make([try Self.problem(422, .referralWindowClosed)], pending: .universalLink)

        let result = await attacher.attachPending(userID: userID)

        XCTAssertEqual(result, ReferralAttachResult(outcome: .refusedClear, toast: "Server message."))
        XCTAssertFalse(attacher.hasPending)
        XCTAssertFalse(attacher.hasAttached(userID: userID))
    }

    func testAnAlreadyAttachedRefusalHidesTheManualField() async throws {
        let (attacher, _, _) = try make([try Self.problem(422, .referralAlreadyAttached)], pending: .universalLink)

        _ = await attacher.attachPending(userID: userID)

        XCTAssertTrue(attacher.hasAttached(userID: userID))
        XCTAssertFalse(attacher.offersManualEntry(userID: userID))
    }

    func testAnUnknownCodeToastsAndClears() async throws {
        let (attacher, _, _) = try make([try Self.problem(404, .referralCodeUnknown)], pending: .universalLink)

        let result = await attacher.attachPending(userID: userID)

        XCTAssertEqual(result?.outcome, .refusedClear)
        XCTAssertFalse(attacher.hasPending)
    }

    func testARetryableFailureKeepsTheCodeAndTheNextForegroundReusesTheKey() async throws {
        let (attacher, transport, _) = try make(
            [
                try Self.problem(503, .upstreamUnavailable), .failure(URLError(.notConnectedToInternet)),
                .json(.created, Self.created),
            ], pending: .clipboard)

        let first = await attacher.attachPending(userID: userID)
        let second = await attacher.attachPending(userID: userID)

        XCTAssertNil(first)
        XCTAssertNil(second)
        XCTAssertTrue(attacher.hasPending)

        let third = await attacher.attachPending(userID: userID)

        XCTAssertEqual(third?.outcome, .attached)
        let keys = await transport.sent.map { Self.key($0) }
        XCTAssertEqual(keys.count, 3)
        XCTAssertEqual(Set(keys).count, 1)
    }

    func testARateLimitKeepsTheCode() async throws {
        let (attacher, _, _) = try make([try Self.problem(429, .rateLimited)], pending: .clipboard)

        let result = await attacher.attachPending(userID: userID)

        XCTAssertNil(result)
        XCTAssertTrue(attacher.hasPending)
    }

    func testNothingIsSentWithoutAPendingReferral() async throws {
        let (attacher, transport, _) = try make([], pending: nil)

        let result = await attacher.attachPending(userID: userID)

        XCTAssertNil(result)
        let count = await transport.sent.count
        XCTAssertEqual(count, 0)
    }

    func testAStaleCodeIsDroppedAndNeverSent() async throws {
        let (attacher, transport, _) = try make(
            [], pending: .clipboard, capturedAt: now.addingTimeInterval(-8 * 86_400))

        let result = await attacher.attachPending(userID: userID)

        XCTAssertNil(result)
        XCTAssertFalse(attacher.hasPending)
        let count = await transport.sent.count
        XCTAssertEqual(count, 0)
    }

    func testAManualAttachSendsTheManualSourceAndReportsAnOfflineFailure() async throws {
        let (attacher, transport, _) = try make(
            [.failure(URLError(.notConnectedToInternet)), .json(.created, Self.created)], pending: nil)
        let code = try XCTUnwrap(ReferralCode("k7m4qx2p"))

        let offline = await attacher.attach(code, userID: userID)
        let joined = await attacher.attach(code, userID: userID)

        XCTAssertEqual(offline, ReferralAttachResult(outcome: .retryLater, toast: "You're offline. Try again."))
        XCTAssertEqual(joined.outcome, .attached)
        let bodies = await transport.sentBodies
        let body = try JSONSerialization.jsonObject(with: try XCTUnwrap(bodies[1])) as? [String: String]
        XCTAssertEqual(body?["source"], "manual")
    }

    func testTheManualFieldIsOfferedOnlyWithoutAPendingOrAttachedReferral() throws {
        let (withPending, _, _) = try make([], pending: .clipboard)
        let (without, _, _) = try make([], pending: nil)

        XCTAssertFalse(withPending.offersManualEntry(userID: userID))
        XCTAssertTrue(without.offersManualEntry(userID: userID))
    }

    private func make(
        _ replies: [StubTransport.Reply], pending source: ReferralSource?, capturedAt: Date? = nil
    ) throws -> (ReferralAttacher, StubTransport, MemoryStore) {
        let store = MemoryStore()
        if let source {
            let code = try XCTUnwrap(ReferralCode("k7m4qx2p"))
            try PendingReferralStore(store: store).save(code, source: source, at: capturedAt ?? now)
        }
        let transport = StubTransport(scripted: replies)
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        let at = now
        return (ReferralAttacher(api: api, store: store, now: { at }), transport, store)
    }

    private static func key(_ request: HTTPRequest) -> String? {
        request.headerFields.first { $0.name.canonicalName == IdempotentSubmission.keyHeader.lowercased() }?.value
    }

    private static func problem(_ status: Int, _ code: Components.Schemas.ErrorCode) throws -> StubTransport.Reply {
        try .problem(
            Components.Schemas.Problem(
                _type: .about_colon_blank, title: "Error", status: status, code: code,
                message: "Server message.", traceId: "4bf92f3577b34da6a3ce929d0e0e4736", retryable: status >= 429))
    }
}
