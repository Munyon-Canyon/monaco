import MonacoAPI
import XCTest

@testable import MonacoCore

#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

final class WithdrawToBalanceTests: XCTestCase {
    override func tearDown() {
        MockURLProtocol.requestHandler = nil
        super.tearDown()
    }

    func testWithdrawToBalance_callsEndpoint() async throws {
        let token = TestFixtures.fixtureSessionToken
        var capturedPath: String?

        MockURLProtocol.requestHandler = { request in
            capturedPath = request.url?.path
            let responseBody = """
                {"id":"job-1","status":"settled","shareUnits":500000,"sliceUsdc":500000,"payoutAddress":"FAKEwallet"}
                """
            let response = HTTPURLResponse(
                url: request.url!,
                statusCode: 200,
                httpVersion: nil,
                headerFields: ["Content-Type": "application/json"]
            )!
            return (response, Data(responseBody.utf8))
        }

        let client = MonacoAPIClient(
            baseURL: URL(string: "https://api.test")!,
            session: makeMockURLSession(),
            accessTokenProvider: { token }
        )

        let job = try await client.withdrawToBalance(groupId: "g1", submission: IdempotentSubmission())
        XCTAssertEqual(capturedPath, "/v1/groups/g1/withdraw-to-balance")
        XCTAssertEqual(job.status, "settled")
        XCTAssertEqual(job.sliceUsdc, 500_000)
    }

    private func makeMockURLSession() -> URLSession {
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [MockURLProtocol.self]
        return URLSession(configuration: configuration)
    }

}
