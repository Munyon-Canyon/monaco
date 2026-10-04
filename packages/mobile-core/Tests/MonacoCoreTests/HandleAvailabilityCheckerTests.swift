import Foundation
import HTTPTypes
import MonacoAPI
import MonacoCore
import MonacoTestClock
import MonacoTestSupport
import XCTest

final class HandleAvailabilityCheckerTests: XCTestCase {
    func testTypingQuicklySendsOneRequestForTheLastInput() async throws {
        let transport = StubTransport(.json(.ok, #"{"handle":"qa_ha","available":true}"#))
        let clock = TestClock()
        let checker = HandleAvailabilityChecker(sessions: Self.sessions(transport), clock: clock)

        for typed in ["q", "qA", "qA_", "qA_h", "qA_hA"] {
            await checker.update(typed)
        }
        await settle(clock)
        clock.advance(by: .milliseconds(399))
        let sentBeforeTheDebounce = await transport.sent.count
        XCTAssertEqual(sentBeforeTheDebounce, 0)
        clock.advance(by: .milliseconds(1))

        let available = await reaches(checker, .available("qa_ha"))
        XCTAssertTrue(available)
        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.path), ["/v1/handles/qa_ha/availability"])
    }

    func testAnInvalidHandleSendsNoRequest() async throws {
        let transport = StubTransport(.json(.ok, #"{"handle":"ad","available":true}"#))
        let clock = TestClock()
        let checker = HandleAvailabilityChecker(sessions: Self.sessions(transport), clock: clock)

        await checker.update("ad")
        clock.advance(by: .seconds(5))

        let status = await checker.status
        XCTAssertEqual(status, .unavailable("ad", .invalid))
        XCTAssertEqual(clock.state.current.requested, [])
        let sent = await transport.sent
        XCTAssertEqual(sent, [])
    }

    func testTheServerReasonBecomesTheStatus() async throws {
        let transport = StubTransport(.json(.ok, #"{"handle":"admin","available":false,"reason":"reserved"}"#))
        let clock = TestClock()
        let checker = HandleAvailabilityChecker(sessions: Self.sessions(transport), clock: clock)

        await checker.update("Admin")
        await settle(clock)
        clock.advance(by: HandleAvailabilityChecker.debounce)

        let reserved = await reaches(checker, .unavailable("admin", .reserved))
        XCTAssertTrue(reserved)
    }

    func testAStaleReplyNeverOverwritesTheNewerInput() async throws {
        let transport = StubTransport(scripted: [
            .gate, .json(.ok, #"{"handle":"abcd","available":true}"#),
        ])
        let clock = TestClock()
        let checker = HandleAvailabilityChecker(sessions: Self.sessions(transport), clock: clock)

        await checker.update("abc")
        await settle(clock)
        clock.advance(by: HandleAvailabilityChecker.debounce)
        await transport.waitForRequest()

        await checker.update("abcd")
        await settle(clock)
        clock.advance(by: HandleAvailabilityChecker.debounce)
        let available = await reaches(checker, .available("abcd"))
        XCTAssertTrue(available)

        await transport.releaseGate(.json(.ok, #"{"handle":"abc","available":false,"reason":"taken"}"#))
        let unchanged = await staysUnchanged(checker, .available("abcd"))

        XCTAssertTrue(unchanged)
        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.path), ["/v1/handles/abc/availability", "/v1/handles/abcd/availability"])
    }

    func testA429SlowsDownAndRetriesAfterRetryAfter() async throws {
        let transport = StubTransport(scripted: [
            try Self.rateLimited(retryAfter: "3"),
            .json(.ok, #"{"handle":"abc","available":true}"#),
        ])
        let clock = TestClock()
        let checker = HandleAvailabilityChecker(sessions: Self.sessions(transport), clock: clock)

        await checker.update("abc")
        await settle(clock)
        clock.advance(by: HandleAvailabilityChecker.debounce)
        let slowed = await reaches(checker, .slowDown("abc"))
        XCTAssertTrue(slowed)
        await settle(clock)
        XCTAssertEqual(clock.state.current.requested.last, .seconds(3))

        clock.advance(by: .milliseconds(2999))
        let sentWhileWaiting = await transport.sent.count
        XCTAssertEqual(sentWhileWaiting, 1)
        clock.advance(by: .milliseconds(1))

        let available = await reaches(checker, .available("abc"))
        XCTAssertTrue(available)
        let sent = await transport.sent.count
        XCTAssertEqual(sent, 2)
    }

    func testATransportFailureWaitsForTryAgain() async throws {
        let transport = StubTransport(scripted: [
            .failure(URLError(.notConnectedToInternet)),
            .json(.ok, #"{"handle":"abc","available":true}"#),
        ])
        let clock = TestClock()
        let checker = HandleAvailabilityChecker(sessions: Self.sessions(transport), clock: clock)

        await checker.update("abc")
        await settle(clock)
        clock.advance(by: HandleAvailabilityChecker.debounce)
        let failed = await reaches(checker, .failed("abc"))
        XCTAssertTrue(failed)
        clock.advance(by: .seconds(60))
        let sentBeforeRetry = await transport.sent.count
        XCTAssertEqual(sentBeforeRetry, 1)

        await checker.retry()

        let available = await reaches(checker, .available("abc"))
        XCTAssertTrue(available)
    }

    func testClearingTheFieldReturnsToIdle() async throws {
        let checker = HandleAvailabilityChecker(
            sessions: Self.sessions(StubTransport(.hang)), clock: TestClock())

        await checker.update("abc")
        await checker.update("  ")

        let status = await checker.status
        XCTAssertEqual(status, .idle)
    }

    private func settle(_ clock: TestClock) async {
        _ = await clock.state.until { $0.pending == 1 }
    }

    private func reaches(_ checker: HandleAvailabilityChecker, _ expected: HandleStatus) async -> Bool {
        await checker.statuses.first { $0 == expected } != nil
    }

    private func staysUnchanged(_ checker: HandleAvailabilityChecker, _ expected: HandleStatus) async -> Bool {
        for _ in 0..<1000 {
            guard await checker.status == expected else { return false }
            await Task.yield()
        }
        return true
    }

    static func sessions(_ transport: StubTransport) -> SessionAPI {
        SessionAPI(
            api: APIClient(
                serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport))
    }

    private static func rateLimited(retryAfter: String) throws -> StubTransport.Reply {
        let problem = Components.Schemas.Problem(
            _type: .about_colon_blank, title: "Too many requests", status: 429, code: .rateLimited,
            message: "Slow down.", traceId: "4bf92f3577b34da6a3ce929d0e0e4736", retryable: true)
        var response = HTTPResponse(status: .tooManyRequests)
        response.headerFields[.contentType] = "application/problem+json"
        response.headerFields[.retryAfter] = retryAfter
        return .response(response, try JSONEncoder().encode(problem))
    }
}

final class HandleSessionAPITests: XCTestCase {
    func testSetHandlePutsTheHandleWithAKeyAndReturnsTheServerProfile() async throws {
        let transport = StubTransport(.json(.ok, Self.meJSON))

        let profile = try await HandleAvailabilityCheckerTests.sessions(transport)
            .setHandle("qa_handle_1", submission: IdempotentSubmission())

        let sent = await transport.sent
        let bodies = await transport.sentBodies
        let keyName = try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
        XCTAssertEqual(sent.map(\.path), ["/v1/me/handle"])
        XCTAssertEqual(sent.map(\.method), [.put])
        XCTAssertNotNil(sent.first?.headerFields[keyName])
        let body = try XCTUnwrap(bodies.first.flatMap { $0 })
        XCTAssertEqual(try JSONSerialization.jsonObject(with: body) as? [String: String], ["handle": "qa_handle_1"])
        XCTAssertEqual(profile.handle, "qa_handle_1")
        XCTAssertEqual(profile.authState, .created)
    }

    func testATakenHandleOnSaveIsTheInlineReason() async throws {
        let transport = try StubTransport.problem(
            Components.Schemas.Problem(
                _type: .about_colon_blank, title: "Conflict", status: 409, code: .handleTaken,
                message: "That handle is taken.", traceId: "4bf92f3577b34da6a3ce929d0e0e4736", retryable: false))

        do {
            _ = try await HandleAvailabilityCheckerTests.sessions(transport)
                .setHandle("kai", submission: IdempotentSubmission())
            XCTFail("expected a problem")
        } catch let error as APIError {
            XCTAssertEqual(HandleSaveFailure(error), .inline(.taken))
        }
    }

    private static let meJSON = """
        {"id":"01890a5d-ac96-774b-bcce-b302099a8058","handle":"qa_handle_1","display_name":"Kai Cenat",\
        "photo_url":null,"auth_state":"CREATED","account_status":"active","member_wallet_address":"wallet-1",\
        "phone_linked":false,"x_username":null,"handle_changeable_at":"2026-10-30T12:00:00Z",\
        "created_at":"2026-09-30T12:00:00Z"}
        """
}
