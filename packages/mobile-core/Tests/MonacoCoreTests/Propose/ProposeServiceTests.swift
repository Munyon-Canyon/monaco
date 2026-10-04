import HTTPTypes
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

final class ProposeServiceTests: XCTestCase {
    private let proposalID = "01890a5d-ac96-774b-bcce-b302099a8057"

    func testWithdrawSendsAndReplaysTheIdempotencyKey() async throws {
        let transport = StubTransport(.json(.internalServerError, "{}"))
        let service = LiveProposeService(api: api(transport))
        let submission = IdempotentSubmission(makeKey: { "withdraw-key" })

        _ = try? await service.withdraw(proposalID: proposalID, submission: submission)
        _ = try? await service.withdraw(proposalID: proposalID, submission: submission)

        let sent = await transport.sent
        let key = try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
        XCTAssertEqual(sent.map(\.method), [.delete, .delete])
        XCTAssertEqual(sent.map(\.path), ["/v1/proposals/\(proposalID)", "/v1/proposals/\(proposalID)"])
        XCTAssertEqual(sent.compactMap { $0.headerFields[key] }, ["withdraw-key", "withdraw-key"])
    }

    private func api(_ transport: StubTransport) -> APIClient {
        APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
    }
}
