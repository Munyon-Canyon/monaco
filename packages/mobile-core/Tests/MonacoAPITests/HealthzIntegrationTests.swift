import Foundation
import MonacoAPI
import MonacoTestSupport
import OpenAPIRuntime
import OpenAPIURLSession
import XCTest

final class HealthzIntegrationTests: XCTestCase {
    func testHealthzAgainstARunningBackend() async throws {
        guard let base = ProcessInfo.processInfo.environment["MONACO_API_URL"], let serverURL = URL(string: base) else {
            throw XCTSkip("integration: set MONACO_API_URL to a running backend")
        }
        let client = APIClient(
            serverURL: serverURL, tokens: StubTokenProvider(token: nil), transport: URLSessionTransport())

        let body = try await client.read { try await $0.getHealthz().ok.body.plainText }

        let text = try await String(collecting: body, upTo: 16)
        XCTAssertEqual(text, "ok\n")
    }
}
