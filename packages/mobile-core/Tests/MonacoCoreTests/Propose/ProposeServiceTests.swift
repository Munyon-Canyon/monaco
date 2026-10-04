import Foundation
import HTTPTypes
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

final class ProposeServiceTests: XCTestCase {
    private let proposalID = "01890a5d-ac96-774b-bcce-b302099a8057"
    private let cabalID = "01890a5d-ac96-774b-bcce-b302099a8058"

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

    func testProposeOmitsAnEmptyThesisAndReplaysItsKey() async throws {
        let transport = StubTransport(.json(.internalServerError, "{}"))
        let service = LiveProposeService(api: api(transport))
        let submission = IdempotentSubmission(makeKey: { "propose-key" })
        let draft = ProposalDraft.buy(symbol: "AAPLx", usdcMicros: 5_000_000, thesis: "  \n")

        _ = try? await service.propose(cabalID: cabalID, draft: draft, submission: submission)
        _ = try? await service.propose(cabalID: cabalID, draft: draft, submission: submission)

        let sent = await transport.sent
        let bodies = await transport.sentBodies.compactMap { $0 }
        let key = try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
        let first = try XCTUnwrap(try JSONSerialization.jsonObject(with: XCTUnwrap(bodies.first)) as? [String: Any])
        XCTAssertEqual(sent.map(\.method), [.post, .post])
        XCTAssertEqual(sent.compactMap { $0.headerFields[key] }, ["propose-key", "propose-key"])
        XCTAssertNil(first["thesis"])
    }

    func testProposeReturnsTheCreatedProposalID() async throws {
        let transport = StubTransport(.json(.created, proposal()))
        let created = try await LiveProposeService(api: api(transport)).propose(
            cabalID: cabalID,
            draft: .sell(symbol: "AAPLx", tokenAmount: 1_000_000, thesis: "Trimmed"),
            submission: IdempotentSubmission()
        )

        XCTAssertEqual(created, proposalID)
    }

    private func api(_ transport: StubTransport) -> APIClient {
        APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
    }

    private func proposal() -> String {
        #"{"id":"\#(proposalID)","cabal_id":"\#(cabalID)","proposer_id":"01890a5d-ac96-774b-bcce-b302099a8059","kind":"sell","symbol":"AAPLx","usdc_micros":null,"token_amount":1000000,"quote_out_amount":5000000,"thesis":"Trimmed","status":"open","status_reason":null,"status_message":null,"expires_at":"2026-10-04T15:00:00Z","created_at":"2026-10-03T15:00:00Z","tally":{"yes":0,"no":0,"voters":1,"needed":1},"my_ballot":null,"voters":[],"can_vote":true,"can_withdraw":true,"swap":null}"#
    }
}
