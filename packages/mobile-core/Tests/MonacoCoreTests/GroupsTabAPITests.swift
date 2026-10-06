import XCTest

@testable import MonacoCore

#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

final class GroupsTabAPITests: XCTestCase {
    override func tearDown() {
        MockURLProtocol.requestHandler = nil
        super.tearDown()
    }

    // MARK: - Error handling

    func testGroupsTabEndpoints_on401_throwHttpStatusError() async throws {
        // Arrange
        MockURLProtocol.requestHandler = { request in
            let response = HTTPURLResponse(
                url: request.url!,
                statusCode: 401,
                httpVersion: nil,
                headerFields: nil
            )!
            return (response, Data())
        }

        let client = MonacoAPIClient(
            baseURL: URL(string: "https://api.test")!,
            session: makeMockURLSession(),
            accessTokenProvider: { TestFixtures.fixtureSessionToken }
        )

        // Act / Assert
        do {
            try await client.castVote(proposalId: "p", choice: "yes")
            XCTFail("Expected httpStatus(401) to be thrown")
        } catch let MonacoAPIError.httpStatus(code, _) {
            XCTAssertEqual(code, 401)
        }
    }

    func testGroupsTabEndpoints_onMalformedJSON_throwDecodingError() async throws {
        // Arrange
        MockURLProtocol.requestHandler = { request in
            let response = HTTPURLResponse(
                url: request.url!,
                statusCode: 200,
                httpVersion: nil,
                headerFields: ["Content-Type": "application/json"]
            )!
            return (response, Data(#"{"groups": [}"#.utf8))
        }

        let client = MonacoAPIClient(
            baseURL: URL(string: "https://api.test")!,
            session: makeMockURLSession(),
            accessTokenProvider: { TestFixtures.fixtureSessionToken }
        )

        // Act / Assert
        do {
            _ = try await client.getGroupView(groupId: "g")
            XCTFail("Expected a DecodingError to be thrown")
        } catch is DecodingError {
            // expected
        }
    }

    private func makeMockURLSession() -> URLSession {
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [MockURLProtocol.self]
        return URLSession(configuration: configuration)
    }
}
