import MonacoAPI
import MonacoTestSupport
import XCTest

@testable import MonacoCore

final class ProposalsRepositoryMappingTests: XCTestCase {
    func testSummaryMapsTheProposalAndPermissions() {
        var proposal = Components.Schemas.Proposal.sample(status: .executed, kind: .sell)
        proposal.statusMessage = "Done"
        let summary = ProposalSummary(proposal, canVote: true, canWithdraw: true)
        XCTAssertEqual(summary.id, "proposal-1")
        XCTAssertEqual(summary.cabalID, "cabal-1")
        XCTAssertEqual(summary.kind, "sell")
        XCTAssertEqual(summary.symbol, "AAPLx")
        XCTAssertEqual(summary.status, .executed)
        XCTAssertEqual(summary.statusMessage, "Done")
        XCTAssertTrue(summary.canVote)
        XCTAssertTrue(summary.canWithdraw)
        XCTAssertNil(summary.swap)
    }

    func testSwapAndPendingVoteMapOptionalAndRequiredFields() {
        let absent = ProposalSwap(nil)
        XCTAssertEqual(absent.status, "")
        XCTAssertFalse(absent.retryable)
        let pending = PendingVote(.init(proposalId: "p", cabalId: "c", kind: .buy, symbol: "TSLAx", expiresAt: .now))
        XCTAssertEqual(pending.id, "p")
        XCTAssertEqual(pending.cabalID, "c")
        XCTAssertEqual(pending.kind, "buy")
        XCTAssertEqual(pending.symbol, "TSLAx")
    }

    func testListReadsTheRequestedFilter() async throws {
        let transport = StubTransport(
            .json(
                .ok,
                #"{"proposals":[{"id":"p","cabal_id":"c","proposer_id":"u","kind":"buy","symbol":"AAPLx","usdc_micros":1,"token_amount":null,"quote_out_amount":1,"thesis":null,"status":"open","status_reason":null,"status_message":null,"expires_at":"2026-01-01T00:00:00Z","created_at":"2025-01-01T00:00:00Z","tally":{"yes":0,"no":0,"voters":1,"needed":1},"my_ballot":null}],"next_cursor":"next"}"#
            ))
        let result = try await repository(transport).list(cabalID: "c", filter: .open, cursor: nil)
        XCTAssertEqual(result.items.map(\.id), ["p"])
        XCTAssertEqual(result.nextCursor, "next")
        let path = await transport.sent.first?.path
        XCTAssertTrue(path?.contains("filter=open") ?? false)
    }

    func testPendingVotesReadTheInbox() async throws {
        let transport = StubTransport(
            .json(
                .ok,
                #"[{"proposal_id":"p","cabal_id":"c","kind":"buy","symbol":"AAPLx","expires_at":"2026-01-01T00:00:00Z"}]"#
            ))
        let pending = try await repository(transport).pendingVotes()
        XCTAssertEqual(pending.map(\.id), ["p"])
        let path = await transport.sent.first?.path
        XCTAssertEqual(path, "/v1/me/pending-votes")
    }

    func testDetailMapsPermissionsAndVoters() async throws {
        let body =
            #"{"id":"p","cabal_id":"c","proposer_id":"u","kind":"buy","symbol":"AAPLx","usdc_micros":1,"token_amount":null,"quote_out_amount":1,"thesis":null,"status":"open","status_reason":null,"status_message":null,"expires_at":"2026-01-01T00:00:00Z","created_at":"2025-01-01T00:00:00Z","tally":{"yes":0,"no":0,"voters":1,"needed":1},"my_ballot":null,"voters":[{"user_id":"u","choice":null,"cast_at":null}],"can_vote":true,"can_withdraw":false,"swap":null}"#
        let detail = try await repository(StubTransport(.json(.ok, body))).detail(id: "p")
        XCTAssertEqual(detail.id, "p")
        XCTAssertEqual(detail.voterIDs, ["u"])
        XCTAssertTrue(detail.summary.canVote)
        XCTAssertFalse(detail.summary.canWithdraw)
    }

    private func repository(_ transport: StubTransport) -> ProposalsRepository {
        ProposalsRepository(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token"), transport: transport))
    }
}
