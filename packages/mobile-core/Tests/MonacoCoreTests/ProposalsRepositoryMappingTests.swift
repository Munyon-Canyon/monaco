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

    @MainActor
    func testListModelLoadsTheFirstPage() async throws {
        let body =
            #"{"proposals":[{"id":"p","cabal_id":"c","proposer_id":"u","kind":"buy","symbol":"AAPLx","usdc_micros":1,"token_amount":null,"quote_out_amount":1,"thesis":null,"status":"open","status_reason":null,"status_message":null,"expires_at":"2026-01-01T00:00:00Z","created_at":"2025-01-01T00:00:00Z","tally":{"yes":0,"no":0,"voters":1,"needed":1},"my_ballot":null}],"next_cursor":null}"#
        let model = ProposalListModel(
            cabalID: "c", filter: .open, repository: repository(StubTransport(.json(.ok, body))),
            hints: FakeHintStream())
        await model.load()
        XCTAssertEqual(model.pager.items.map(\.id), ["p"])
        model.setVisible(true)
    }

    @MainActor
    func testDetailModelLoadsAProposal() async throws {
        let body = detailBody
        let model = ProposalDetailModel(
            id: "p", cabalID: "c", repository: repository(StubTransport(.json(.ok, body))), hints: FakeHintStream())
        await model.load()
        XCTAssertEqual(model.value?.id, "p")
        XCTAssertNil(model.errorMessage)
        model.setVisible(true)
    }

    @MainActor
    func testDetailModelShowsAnOfflineError() async throws {
        let model = ProposalDetailModel(
            id: "p", cabalID: "c",
            repository: repository(StubTransport(.failure(URLError(.notConnectedToInternet)))), hints: FakeHintStream())
        await model.load()
        XCTAssertNil(model.value)
        XCTAssertEqual(model.errorMessage, "You're offline. Try again.")
    }

    @MainActor
    func testDetailModelVotesThenReloads() async throws {
        let transport = StubTransport(scripted: [
            .json(
                .ok,
                #"{"proposal_id":"p","status":"open","tally":{"yes":1,"no":0,"voters":1,"needed":1},"my_ballot":"yes"}"#
            ),
            .json(.ok, detailBody),
        ])
        let model = ProposalDetailModel(
            id: "p", cabalID: "c", repository: repository(transport), hints: FakeHintStream())
        await model.vote("yes")
        XCTAssertEqual(model.value?.id, "p")
        XCTAssertNil(model.errorMessage)
    }

    @MainActor
    func testDetailModelReloadsForAProposalHint() async throws {
        let transport = StubTransport(scripted: [.json(.ok, detailBody), .json(.ok, detailBody)])
        let hints = FakeHintStream()
        let model = ProposalDetailModel(id: "p", cabalID: "c", repository: repository(transport), hints: hints)
        let observer = Task { await model.observe() }
        addTeardownBlock { observer.cancel() }
        let subscribed = await waitUntil { await hints.subscriberCount == 1 }
        XCTAssertTrue(subscribed)
        model.setVisible(true)
        await hints.send(.changed(.cabal("c"), what: "proposal_updated", id: "1"))
        let refreshed = await waitUntil { await transport.sent.count == 1 }
        XCTAssertTrue(refreshed)
    }

    @MainActor
    func testPendingModelLoadsVotesAndDetails() async throws {
        let transport = StubTransport(scripted: [
            .json(
                .ok,
                #"[{"proposal_id":"p","cabal_id":"c","kind":"buy","symbol":"AAPLx","expires_at":"2026-01-01T00:00:00Z"}]"#
            ),
            .json(.ok, detailBody),
        ])
        let model = PendingVotesModel(repository: repository(transport), hints: FakeHintStream())
        await model.load()
        XCTAssertEqual(model.votes.map(\.id), ["p"])
        XCTAssertEqual(model.details["p"]?.id, "p")
        model.setVisible(true)
    }

    @MainActor
    func testPendingModelKeepsVotesWhenAProposalDetailCannotLoad() async throws {
        let transport = StubTransport(scripted: [
            .json(
                .ok,
                #"[{"proposal_id":"p","cabal_id":"c","kind":"buy","symbol":"AAPLx","expires_at":"2026-01-01T00:00:00Z"}]"#
            ),
            .failure(URLError(.notConnectedToInternet)),
        ])
        let model = PendingVotesModel(repository: repository(transport), hints: FakeHintStream())
        await model.load()
        XCTAssertEqual(model.votes.map(\.id), ["p"])
        XCTAssertTrue(model.details.isEmpty)
    }

    @MainActor
    func testPendingModelRetainsItsStateWhenTheInboxFails() async throws {
        let model = PendingVotesModel(
            repository: repository(StubTransport(.failure(URLError(.notConnectedToInternet)))), hints: FakeHintStream())
        await model.load()
        XCTAssertTrue(model.votes.isEmpty)
        XCTAssertTrue(model.details.isEmpty)
    }

    @MainActor
    func testListModelReloadsForAProposalCreatedHint() async throws {
        let body = listBody
        let transport = StubTransport(scripted: [.json(.ok, body), .json(.ok, body)])
        let hints = FakeHintStream()
        let model = ProposalListModel(cabalID: "c", filter: .open, repository: repository(transport), hints: hints)
        let observer = Task { await model.observe(cabalID: "c") }
        addTeardownBlock { observer.cancel() }
        let subscribed = await waitUntil { await hints.subscriberCount == 1 }
        XCTAssertTrue(subscribed)
        model.setVisible(true)
        await hints.send(.changed(.cabal("c"), what: "proposal_created", id: "1"))
        let refreshed = await waitUntil { await transport.sent.count == 1 }
        XCTAssertTrue(refreshed)
    }

    private var listBody: String {
        #"{"proposals":[{"id":"p","cabal_id":"c","proposer_id":"u","kind":"buy","symbol":"AAPLx","usdc_micros":1,"token_amount":null,"quote_out_amount":1,"thesis":null,"status":"open","status_reason":null,"status_message":null,"expires_at":"2026-01-01T00:00:00Z","created_at":"2025-01-01T00:00:00Z","tally":{"yes":0,"no":0,"voters":1,"needed":1},"my_ballot":null}],"next_cursor":null}"#
    }

    private var detailBody: String {
        #"{"id":"p","cabal_id":"c","proposer_id":"u","kind":"buy","symbol":"AAPLx","usdc_micros":1,"token_amount":null,"quote_out_amount":1,"thesis":null,"status":"open","status_reason":null,"status_message":null,"expires_at":"2026-01-01T00:00:00Z","created_at":"2025-01-01T00:00:00Z","tally":{"yes":0,"no":0,"voters":1,"needed":1},"my_ballot":null,"voters":[{"user_id":"u","choice":null,"cast_at":null}],"can_vote":true,"can_withdraw":false,"swap":null}"#
    }

    private func repository(_ transport: StubTransport) -> ProposalsRepository {
        ProposalsRepository(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token"), transport: transport))
    }

    @MainActor
    private func waitUntil(_ condition: @escaping @MainActor () async -> Bool) async -> Bool {
        for _ in 0..<100 {
            if await condition() { return true }
            await Task.yield()
        }
        return await condition()
    }
}
