import HTTPTypes
import MonacoTestSupport
import XCTest

final class StubTransportTests: XCTestCase {
    func testWaitForRequestReturnsForAnExistingAndTheNextRequest() async throws {
        let existing = StubTransport.ok("ok")
        _ = try await existing.send(Self.request, body: nil, baseURL: testServerURL, operationID: "test")
        await existing.waitForRequest()

        let next = StubTransport.ok("ok")
        let waiter = Task { await next.waitForRequest() }
        _ = try await next.send(Self.request, body: nil, baseURL: testServerURL, operationID: "test")
        await waiter.value
    }

    private static let request = HTTPRequest(
        method: .get, scheme: "http", authority: "api.test", path: "/v1/test"
    )
}
