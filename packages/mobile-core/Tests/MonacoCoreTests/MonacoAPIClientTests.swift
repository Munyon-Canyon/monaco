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
        try await client.castVote(proposalId: "p", choice: "yes")

        // Assert
        XCTAssertEqual(capturedAuthorization, expectedAuthorization)
    }

    private func makeMockURLSession() -> URLSession {
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [MockURLProtocol.self]
        return URLSession(configuration: configuration)
    }
}
