import MonacoAPI
import XCTest

@testable import MonacoCore

#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

final class ProposalsAPITests: XCTestCase {
    override func tearDown() {
        MockURLProtocol.requestHandler = nil
        super.tearDown()
    }

    func testAPIClient_castVote_postsTheChoiceToTheProposal() async throws {
        let token = TestFixtures.fixtureSessionToken
        var captured: URLRequest?
        var body: Data?
        MockURLProtocol.requestHandler = { request in
            captured = request
            body = Self.httpBody(from: request)
            let url = try XCTUnwrap(request.url)
            let response = try XCTUnwrap(
                HTTPURLResponse(url: url, statusCode: 204, httpVersion: nil, headerFields: nil))
            return (response, Data())
        }
        let client = MonacoAPIClient(
            baseURL: try XCTUnwrap(URL(string: "https://api.test")),
            session: makeMockURLSession(),
            accessTokenProvider: { token }
        )

        try await client.castVote(proposalId: "prop-1", choice: "yes")

        XCTAssertEqual(captured?.url?.path, "/v1/proposals/prop-1/votes")
        XCTAssertEqual(captured?.httpMethod, "POST")
        XCTAssertEqual(captured?.value(forHTTPHeaderField: "Authorization"), "Bearer \(token)")
        let json = try XCTUnwrap(body.flatMap { try? JSONSerialization.jsonObject(with: $0) as? [String: String] })
        XCTAssertEqual(json, ["choice": "yes"])
    }

    private func makeMockURLSession() -> URLSession {
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [MockURLProtocol.self]
        return URLSession(configuration: configuration)
    }

    private static func httpBody(from request: URLRequest) -> Data? {
        if let body = request.httpBody {
            return body
        }
        guard let stream = request.httpBodyStream else {
            return nil
        }
        stream.open()
        defer { stream.close() }
        var data = Data()
        let bufferSize = 1024
        let buffer = UnsafeMutablePointer<UInt8>.allocate(capacity: bufferSize)
        defer { buffer.deallocate() }
        while stream.hasBytesAvailable {
            let read = stream.read(buffer, maxLength: bufferSize)
            if read > 0 {
                data.append(buffer, count: read)
            }
        }
        return data.isEmpty ? nil : data
    }
}
