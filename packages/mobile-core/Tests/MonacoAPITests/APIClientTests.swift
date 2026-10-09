import HTTPTypes
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

final class APIClientTests: XCTestCase {
    func testDefaultRejectedTokenTerminationEndsTheSession() async {
        let token = DefaultToken()
        await token.endSession(rejectedToken: "stale")
        let ended = await token.ended
        XCTAssertTrue(ended)
    }

    func testRequestsCarryTheBearerToken() async throws {
        let transport = StubTransport.ok("ok\n")

        try await Fixtures.client(transport).healthz()

        let sent = await transport.sent
        XCTAssertEqual(sent.map { $0.headerFields[.authorization] }, [nil])
    }

    func testA401IsRetriedOnceWithTheRefreshedToken() async throws {
        let transport = StubTransport(scripted: [Fixtures.problem(401, "unauthorized"), Fixtures.ping])
        let tokens = StubTokenProvider(token: "stale", refreshes: ["fresh"])

        _ = try await Fixtures.client(transport, tokens: tokens).ping(IdempotentSubmission())

        let sent = await transport.sent
        XCTAssertEqual(sent.map { $0.headerFields[.authorization] }, ["Bearer stale", "Bearer fresh"])
        let refreshed = await tokens.refreshed
        XCTAssertEqual(refreshed, ["stale"])
    }

    func testASecond401SignsOut() async throws {
        let transport = StubTransport(scripted: [
            Fixtures.problem(401, "unauthorized"), Fixtures.problem(401, "unauthorized"),
        ])
        let tokens = StubTokenProvider(token: "stale", refreshes: ["fresh", "fresher"])

        await assertThrows(.signedOut) {
            _ = try await Fixtures.client(transport, tokens: tokens).ping(IdempotentSubmission())
        }

        let sent = await transport.sent
        XCTAssertEqual(sent.count, 2)
        let refreshed = await tokens.refreshed
        XCTAssertEqual(refreshed, ["stale"])
        let ended = await tokens.ended
        XCTAssertEqual(ended, ["fresh"])
    }

    func testANilRefreshedTokenSignsOutWithoutResending() async throws {
        let transport = StubTransport(scripted: [Fixtures.problem(401, "unauthorized")])
        let tokens = StubTokenProvider(token: "stale")

        await assertThrows(.signedOut) {
            _ = try await Fixtures.client(transport, tokens: tokens).ping(IdempotentSubmission())
        }

        let sent = await transport.sent
        XCTAssertEqual(sent.count, 1)
    }

    func testABearerRequiredCallWithNoTokenIsNotSentAndDoesNotEndTheSession() async throws {
        let transport = StubTransport(scripted: [Fixtures.problem(401, "unauthorized")])
        let tokens = StubTokenProvider(token: nil, refreshes: ["fresh"])

        await assertThrows(.missingAccessToken("getMe")) {
            _ = try await SessionAPI(api: Fixtures.client(transport, tokens: tokens)).me()
        }

        let refreshed = await tokens.refreshed
        XCTAssertEqual(refreshed, [])
        let unsignedEndings = await tokens.unsignedEndings
        XCTAssertEqual(unsignedEndings, 0)
        let sent = await transport.sent
        XCTAssertTrue(sent.isEmpty)
    }

    func testOnlyHealthzMayRunWithoutABearer() async throws {
        let transport = StubTransport.ok("ok\n")
        let client = Fixtures.client(transport, tokens: StubTokenProvider(token: nil))

        await assertThrows(.missingAccessToken("postAuthSession")) {
            _ = try await SessionAPI(api: client).openSession()
        }
        let unsignedSent = await transport.sent
        XCTAssertTrue(unsignedSent.isEmpty)

        try await client.healthz()
        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.path), ["/healthz"])
        XCTAssertNil(sent[0].headerFields[.authorization])
    }

    func testARefreshedWriteKeepsItsIdempotencyKey() async throws {
        let transport = StubTransport(scripted: [Fixtures.problem(401, "unauthorized"), Fixtures.ping])
        let tokens = StubTokenProvider(token: "stale", refreshes: ["fresh"])

        _ = try await Fixtures.client(transport, tokens: tokens).ping(IdempotentSubmission { "key-1" })

        let keys = await transport.idempotencyKeys
        XCTAssertEqual(keys, ["key-1", "key-1"])
    }

    func testAProblemIsThrownAsAPIError() async throws {
        let transport = StubTransport(Fixtures.problem(404, "not_found", message: "No such cabal."))

        do {
            try await Fixtures.client(transport).healthz()
            XCTFail("expected a throw")
        } catch let APIError.problem(problem) {
            XCTAssertEqual(problem.code, .known(.notFound))
            XCTAssertEqual(problem.message, "No such cabal.")
        }
    }

    func testAnAccountDeletedAnswerEndsTheSessionNamingTheTokenSent() async throws {
        let transport = StubTransport(Fixtures.problem(403, "account_deleted"))
        let tokens = AccountDeletedRecorder(token: "t1")
        let client = APIClient(serverURL: testServerURL, tokens: tokens, transport: transport)

        await assertThrows(.accountDeleted) { _ = try await client.ping(IdempotentSubmission()) }

        let deleted = await tokens.deleted
        XCTAssertEqual(deleted, ["t1"])
    }

    func testAnAccountDeletedAnswerEndsTheSessionOfAProviderThatOnlyEndsSessions() async throws {
        let transport = StubTransport(Fixtures.problem(403, "account_deleted"))
        let tokens = StubTokenProvider(token: "t1")

        await assertThrows(.accountDeleted) {
            _ = try await Fixtures.client(transport, tokens: tokens).ping(IdempotentSubmission())
        }

        let ended = await tokens.ended
        XCTAssertEqual(ended, ["t1"])
    }

    func testAccountDeletedToTheDeleteCallEndsNothing() async throws {
        let transport = StubTransport(Fixtures.problem(403, "account_deleted"))
        let tokens = AccountDeletedRecorder(token: "t1")
        let client = APIClient(serverURL: testServerURL, tokens: tokens, transport: transport)

        await assertThrows(.accountDeleted) {
            try await SessionAPI(api: client).deleteAccount(submission: IdempotentSubmission())
        }

        let deleted = await tokens.deleted
        XCTAssertEqual(deleted, [])
    }

    func testAnotherProblemEndsNothing() async throws {
        let transport = StubTransport(Fixtures.problem(404, "not_found"))
        let tokens = AccountDeletedRecorder(token: "t1")
        let client = APIClient(serverURL: testServerURL, tokens: tokens, transport: transport)

        _ = try? await client.ping(IdempotentSubmission())

        let deleted = await tokens.deleted
        XCTAssertEqual(deleted, [])
    }
}

private actor AccountDeletedRecorder: MonacoAPI.AccessTokenProvider {
    private let token: String
    private(set) var deleted: [String] = []

    init(token: String) { self.token = token }

    func accessToken() async throws -> String? { token }
    func refreshedToken(replacing _: String) async throws -> String? { nil }
    func accountDeleted(rejectedToken: String) { deleted.append(rejectedToken) }
}

private actor DefaultToken: MonacoAPI.AccessTokenProvider {
    private(set) var ended = false
    func accessToken() async throws -> String? { nil }
    func refreshedToken(replacing _: String) async throws -> String? { nil }
    func endSession() { ended = true }
}

func assertThrows(
    _ expected: APIError,
    file: StaticString = #filePath,
    line: UInt = #line,
    _ call: () async throws -> Void
) async {
    do {
        try await call()
        XCTFail("expected \(expected), nothing was thrown", file: file, line: line)
    } catch {
        XCTAssertEqual(error as? APIError, expected, file: file, line: line)
    }
}
