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

    // MARK: - groupLeaderboard

    func testGroupLeaderboard_sendsLimit_andDecodesResponse() async throws {
        // Arrange
        let token = TestFixtures.fixtureSessionToken
        var capturedPath: String?
        var capturedQueryItems: [URLQueryItem]?
        var capturedAuthorization: String?

        let fixtureURL = try XCTUnwrap(
            Bundle.module.url(forResource: "groups_leaderboard", withExtension: "json")
        )
        let fixtureData = try Data(contentsOf: fixtureURL)

        MockURLProtocol.requestHandler = { request in
            capturedPath = request.url?.path
            capturedQueryItems = URLComponents(url: request.url!, resolvingAgainstBaseURL: false)?.queryItems
            capturedAuthorization = request.value(forHTTPHeaderField: "Authorization")
            let response = HTTPURLResponse(
                url: request.url!,
                statusCode: 200,
                httpVersion: nil,
                headerFields: ["Content-Type": "application/json"]
            )!
            return (response, fixtureData)
        }

        let client = MonacoAPIClient(
            baseURL: URL(string: "https://api.test")!,
            session: makeMockURLSession(),
            accessTokenProvider: { token }
        )

        // Act
        let result = try await client.groupLeaderboard(limit: 50)

        // Assert
        XCTAssertEqual(capturedPath, "/v1/groups/leaderboard")
        XCTAssertEqual(capturedQueryItems, [URLQueryItem(name: "limit", value: "50")])
        XCTAssertEqual(capturedAuthorization, "Bearer \(token)")
        XCTAssertEqual(result.groups.count, 2)
        XCTAssertEqual(result.groups[0].rank, 1)
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
            _ = try await client.groupLeaderboard()
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
            _ = try await client.groupLeaderboard()
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
