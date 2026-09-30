import Foundation
import HTTPTypes
@testable import MonacoAPI
import MonacoTestSupport
import OpenAPIRuntime
import XCTest

final class HeadersMiddlewareTests: XCTestCase {
    private func send(
        _ method: HTTPRequest.Method,
        headers: HTTPFields = [:],
        accessToken: @escaping @Sendable () async throws -> String? = { nil }
    ) async throws -> HTTPRequest {
        let transport = StubTransport.ok("ok\n")
        let request = HTTPRequest(method: method, scheme: "http", authority: "api.test", path: "/v1/x", headerFields: headers)
        _ = try await HeadersMiddleware(accessToken: accessToken).intercept(
            request,
            body: nil,
            baseURL: testServerURL,
            operationID: "x"
        ) { request, body, url in
            try await transport.send(request, body: body, baseURL: url, operationID: "x")
        }
        let sent = await transport.sent
        return try XCTUnwrap(sent.first)
    }

    func testSignedInRequestsCarryTheBearerToken() async throws {
        let request = try await send(.get, accessToken: { "token-1" })

        XCTAssertEqual(request.headerFields[.authorization], "Bearer token-1")
    }

    func testSignedOutRequestsCarryNoAuthorization() async throws {
        let request = try await send(.get)

        XCTAssertNil(request.headerFields[.authorization])
    }

    func testAWriteInsideASubmissionCarriesItsKey() async throws {
        let request = try await IdempotencyKey.$current.withValue("key-1") { try await send(.post) }

        XCTAssertEqual(request.headerFields[IdempotencyKey.header], "key-1")
    }

    func testAReadInsideASubmissionCarriesNoKey() async throws {
        let request = try await IdempotencyKey.$current.withValue("key-1") { try await send(.get) }

        XCTAssertNil(request.headerFields[IdempotencyKey.header])
    }

    func testAWriteOutsideASubmissionCarriesNoKey() async throws {
        let request = try await send(.post)

        XCTAssertNil(request.headerFields[IdempotencyKey.header])
    }

    func testAKeyTheCallerSetIsKept() async throws {
        let request = try await IdempotencyKey.$current.withValue("key-1") {
            try await send(.post, headers: [IdempotencyKey.header: "explicit"])
        }

        XCTAssertEqual(request.headerFields[IdempotencyKey.header], "explicit")
    }
}
