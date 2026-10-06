import MonacoAPI
import XCTest

@testable import MonacoCore

#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

final class MonacoAPIClientTests: XCTestCase {
    override func tearDown() {
        MockURLProtocol.requestHandler = nil
        super.tearDown()
    }

    func testAPIClient_afterLogin_sendsAuthorizationHeader() async throws {
        // Arrange
        let token = TestFixtures.fixtureSessionToken
        let expectedAuthorization = "Bearer \(token)"
        var capturedAuthorization: String?

        MockURLProtocol.requestHandler = { request in
            capturedAuthorization = request.value(forHTTPHeaderField: "Authorization")
            let responseBody = """
                {}
                """
            let response = HTTPURLResponse(
                url: request.url!,
                statusCode: 200,
                httpVersion: nil,
                headerFields: ["Content-Type": "application/json"]
            )!
            return (response, Data(responseBody.utf8))
        }

        let session = makeMockURLSession()
        let client = MonacoAPIClient(
            baseURL: URL(string: "https://api.test")!,
            session: session,
            accessTokenProvider: { token }
        )

        // Act
        _ = try await client.getHomeDashboard()

        // Assert
        XCTAssertEqual(capturedAuthorization, expectedAuthorization)
    }

    func testAPIClient_getHomeDashboard_callsV1HomeDashboard() async throws {
        // Arrange
        var capturedPath: String?
        var capturedQuery: String?

        MockURLProtocol.requestHandler = { request in
            capturedPath = request.url?.path
            capturedQuery = request.url?.query
            let response = HTTPURLResponse(
                url: request.url!,
                statusCode: 200,
                httpVersion: nil,
                headerFields: ["Content-Type": "application/json"]
            )!
            return (response, Data("{}".utf8))
        }

        let client = MonacoAPIClient(
            baseURL: URL(string: "https://api.test")!,
            session: makeMockURLSession(),
            accessTokenProvider: { TestFixtures.fixtureSessionToken }
        )

        // Act
        let dashboard = try await client.getHomeDashboard()

        // Assert
        XCTAssertEqual(capturedPath, "/v1/home/dashboard")
        XCTAssertNil(capturedQuery)
        XCTAssertEqual(dashboard, HomeDashboardDTO())
    }

    private func makeMockURLSession() -> URLSession {
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [MockURLProtocol.self]
        return URLSession(configuration: configuration)
    }
}
