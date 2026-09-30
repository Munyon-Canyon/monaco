import Foundation
import HTTPTypes
@testable import MonacoAPI
import MonacoTestSupport
import OpenAPIRuntime
import XCTest

final class HeadersMiddlewareTests: XCTestCase {
    private func send(
        _ method: HTTPRequest.Method,
        accessToken: @escaping @Sendable () async throws -> String? = { nil }
    ) async throws -> HTTPRequest {
        let transport = StubTransport.ok("ok\n")
        let request = HTTPRequest(method: method, scheme: "http", authority: "api.test", path: "/v1/x")
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
}
