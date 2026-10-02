import HTTPTypes
import MonacoAPI
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
        XCTAssertEqual(sent.map { $0.headerFields[.authorization] }, ["Bearer token-1"])
    }

    func testA401IsRetriedOnceWithTheRefreshedToken() async throws {
        let transport = StubTransport(scripted: [Fixtures.problem(401, "unauthorized"), .ok("ok\n")])
        let tokens = StubTokenProvider(token: "stale", refreshes: ["fresh"])

        try await Fixtures.client(transport, tokens: tokens).healthz()

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

        await assertThrows(.signedOut) { try await Fixtures.client(transport, tokens: tokens).healthz() }

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

        await assertThrows(.signedOut) { try await Fixtures.client(transport, tokens: tokens).healthz() }

        let sent = await transport.sent
        XCTAssertEqual(sent.count, 1)
    }

    func testA401WithoutATokenSignsOutWithoutRefreshing() async throws {
        let transport = StubTransport(scripted: [Fixtures.problem(401, "unauthorized")])
        let tokens = StubTokenProvider(token: nil, refreshes: ["fresh"])

        await assertThrows(.signedOut) { try await Fixtures.client(transport, tokens: tokens).healthz() }

        let refreshed = await tokens.refreshed
        XCTAssertEqual(refreshed, [])
        let unsignedEndings = await tokens.unsignedEndings
        XCTAssertEqual(unsignedEndings, 1)
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
}

private actor DefaultToken: AccessTokenProvider {
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
