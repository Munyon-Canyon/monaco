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
                #"{"proposals":[{"id":"p","cabal_id":"c","proposer_id":"u","kind":"buy","symbol":"AAPLx","usdc_micros":1,"token_amount":null,"quote_out_amount":1,"thesis":null,"status":"open","status_reason":null,"status_message":null,"expires_at":"2026-01-01T00:00:00Z","created_at":"2025-01-01T00:00:00Z","tally":{"yes":0,"no":0,"voters":1,"needed":1},"my_ballot":null,"can_vote":false}],"next_cursor":"next"}"#
            ))
        let result = try await repository(transport).list(cabalID: "c", filter: .open, cursor: nil)
        XCTAssertEqual(result.items.map(\.id), ["p"])
        XCTAssertEqual(result.items.map(\.canVote), [false])
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
        XCTAssertEqual(detail.voters.map(\.id), ["u"])
        XCTAssertNil(detail.voters.first?.ballot)
        XCTAssertEqual(detail.summary.proposerID, "u")
        XCTAssertEqual(detail.summary.tally.needed, 1)
        XCTAssertTrue(detail.summary.canVote)
        XCTAssertFalse(detail.summary.canWithdraw)
    }

    func testDetailKeepsTheCallersBallot() async throws {
        let body = detailBody.replacingOccurrences(of: #""my_ballot":null"#, with: #""my_ballot":"yes""#)
        let detail = try await repository(StubTransport(.json(.ok, body))).detail(id: "p")
        XCTAssertEqual(detail.summary.myBallot, "yes")
    }

    func testMembersAndAssetExposeScreenMetadata() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, cabalBody),
            .json(.ok, assetBody),
        ])
        let repository = repository(transport)
        let members = try await repository.members(cabalID: "c")
        let asset = try await repository.asset(symbol: "AAPLx")
        XCTAssertEqual(members.map(\.name), ["Jordan"])
        XCTAssertEqual(asset.displayName, "Apple")
        XCTAssertEqual(asset.kind, .stock)
        XCTAssertEqual(asset.decimals, 8)
    }

    @MainActor
    func testListModelLoadsTheFirstPage() async throws {
        let body =
            #"{"proposals":[{"id":"p","cabal_id":"c","proposer_id":"u","kind":"buy","symbol":"AAPLx","usdc_micros":1,"token_amount":null,"quote_out_amount":1,"thesis":null,"status":"open","status_reason":null,"status_message":null,"expires_at":"2026-01-01T00:00:00Z","created_at":"2025-01-01T00:00:00Z","tally":{"yes":0,"no":0,"voters":1,"needed":1},"my_ballot":null,"can_vote":false}],"next_cursor":null}"#
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
    func testDetailModelHidesWithdrawWhenTheServerDisallowsIt() async throws {
        let model = ProposalDetailModel(
            id: "p", cabalID: "c", repository: repository(StubTransport(.json(.ok, detailBody))),
            hints: FakeHintStream())
        await model.load()
        XCTAssertFalse(model.canWithdraw)
    }

    @MainActor
    func testDetailModelWithdrawReplaysItsIdempotencyKeyAfterAnError() async throws {
        let transport = StubTransport(.json(.internalServerError, #"{"message":"Try again"}"#))
        let model = ProposalDetailModel(
            id: "p", cabalID: "c", repository: repository(transport), hints: FakeHintStream())
        await model.withdraw()
        await model.withdraw()
        let key = try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
        let sent = await transport.sent
        let keys = sent.compactMap { $0.headerFields[key] }
        XCTAssertEqual(keys.count, 2)
        XCTAssertEqual(keys.first, keys.last)
    }

    @MainActor
    func testDetailModelRecordsWithdrawThenRefreshesItsState() async throws {
        let withdrawn = detailBody.replacingOccurrences(of: #""status":"open""#, with: #""status":"withdrawn""#)
        let transport = StubTransport(scripted: [
            .json(.ok, withdrawn), .json(.ok, withdrawn), .json(.ok, cabalBody), .json(.ok, assetBody),
        ])
        let model = ProposalDetailModel(
            id: "p", cabalID: "c", repository: repository(transport), hints: FakeHintStream())
        await model.withdraw()
        XCTAssertTrue(model.didWithdraw)
        XCTAssertEqual(model.value?.summary.status, .withdrawn)
    }

    @MainActor
    func testDetailModelVotesThenReloads() async throws {
        let transport = StubTransport(
            scripted: loadReplies(detailBody) + [.json(.ok, voteResult("yes"))] + loadReplies(detailBody))
        let model = ProposalDetailModel(
            id: "p", cabalID: "c", repository: repository(transport), hints: FakeHintStream())
        await model.load()
        let voted = await model.vote("yes")
        XCTAssertTrue(voted)
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
        let subscribed = await eventArrives(within: 30) { await hints.waitForSubscribers(2) }
        XCTAssertTrue(subscribed)
        model.setVisible(true)
        await hints.send(.changed(.cabal("c"), what: "proposal_updated", id: "1"))
        let refreshed = await eventArrives(within: 30) { await transport.waitForRequest() }
        XCTAssertTrue(refreshed)
    }

    @MainActor
    func testPendingModelLoadsVotesAndDetails() async throws {
        let transport = StubTransport(routes: [
            "/v1/me/pending-votes": [
                .json(
                    .ok,
                    #"[{"proposal_id":"p","cabal_id":"c","kind":"buy","symbol":"AAPLx","expires_at":"2026-01-01T00:00:00Z"}]"#
                )
            ],
            "/v1/proposals/p": [.json(.ok, detailBody)],
        ])
        let model = PendingVotesModel(repository: repository(transport), hints: FakeHintStream())
        await model.load()
        XCTAssertEqual(model.votes.map(\.id), ["p"])
        XCTAssertEqual(model.details["p"]?.id, "p")
        model.setVisible(true)
    }

    @MainActor
    func testPendingModelKeepsVotesWhenAProposalDetailCannotLoad() async throws {
        let transport = StubTransport(routes: [
            "/v1/me/pending-votes": [
                .json(
                    .ok,
                    #"[{"proposal_id":"p","cabal_id":"c","kind":"buy","symbol":"AAPLx","expires_at":"2026-01-01T00:00:00Z"}]"#
                )
            ],
            "/v1/proposals/p": [.failure(URLError(.notConnectedToInternet))],
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
        let subscribed = await eventArrives(within: 30) { await hints.waitForSubscribers(3) }
        XCTAssertTrue(subscribed)
        model.setVisible(true)
        await hints.send(.changed(.cabal("c"), what: "proposal_created", id: "1"))
        let refreshed = await eventArrives(within: 30) { await transport.waitForRequest() }
        XCTAssertTrue(refreshed)
    }

    private var listBody: String {
        #"{"proposals":[{"id":"p","cabal_id":"c","proposer_id":"u","kind":"buy","symbol":"AAPLx","usdc_micros":1,"token_amount":null,"quote_out_amount":1,"thesis":null,"status":"open","status_reason":null,"status_message":null,"expires_at":"2026-01-01T00:00:00Z","created_at":"2025-01-01T00:00:00Z","tally":{"yes":0,"no":0,"voters":1,"needed":1},"my_ballot":null,"can_vote":false}],"next_cursor":null}"#
    }

    private var cabalBody: String {
        #"{"id":"c","name":"Cabal","picture_url":null,"status":"active","rules":{"join_mode":"request","voter_mode":"all","threshold":"majority","proposal_expiry_seconds":86400,"slippage_bps":100},"creator":{"user_id":"u","handle":"jordan","display_name":"Jordan","photo_url":null},"member_count":1,"members":[{"user_id":"u","handle":"jordan","display_name":"Jordan","photo_url":null,"role":"creator","can_vote":true,"joined_at":"2026-01-01T00:00:00Z"}],"me":{"role":"creator","can_vote":true},"my_access_request":null,"invite_code":null,"treasury_address":"treasury"}"#
    }

    private var assetBody: String {
        #"{"symbol":"AAPLx","display_name":"Apple","issuer":"xstocks","kind":"equity","logo_url":null,"price_micros":null,"price_as_of":null,"change_bps":null,"sparkline_micros":null,"session":{"state":"open","continuous":false,"holiday":"","early_close":false,"next_state":null,"next_transition":null},"decimals":8,"ui_multiplier":{"num":1,"den":1},"tradable":true,"other_listings":[],"attribution":"test"}"#
    }

    private var detailBody: String {
        #"{"id":"p","cabal_id":"c","proposer_id":"u","kind":"buy","symbol":"AAPLx","usdc_micros":1,"token_amount":null,"quote_out_amount":1,"thesis":null,"status":"open","status_reason":null,"status_message":null,"expires_at":"2026-01-01T00:00:00Z","created_at":"2025-01-01T00:00:00Z","tally":{"yes":0,"no":0,"voters":1,"needed":1},"my_ballot":null,"voters":[{"user_id":"u","choice":null,"cast_at":null}],"can_vote":true,"can_withdraw":false,"swap":null}"#
    }

    private func loadReplies(_ detail: String) -> [StubTransport.Reply] {
        [.json(.ok, detail), .json(.ok, "{}"), .json(.ok, "{}")]
    }

    private func voteResult(_ choice: String) -> String {
        #"{"proposal_id":"p","status":"open","tally":{"yes":1,"no":0,"voters":1,"needed":1},"my_ballot":"\#(choice)"}"#
    }

    private func repository(_ transport: StubTransport) -> ProposalsRepository {
        ProposalsRepository(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token"), transport: transport))
    }
}

extension ProposalsRepositoryMappingTests {
    @MainActor
    func testDetailModelReportsAFailedVote() async throws {
        let transport = StubTransport(
            scripted: loadReplies(detailBody) + [.failure(URLError(.notConnectedToInternet))])
        let model = ProposalDetailModel(
            id: "p", cabalID: "c", repository: repository(transport), hints: FakeHintStream())
        await model.load()
        let voted = await model.vote("no")
        XCTAssertFalse(voted)
        XCTAssertFalse(model.isVoting)
        XCTAssertNil(model.summary?.myBallot)
        XCTAssertEqual(model.errorMessage, "You're offline. Try again.")
    }

    @MainActor
    func testDetailModelReportsAVoteWhoseReloadFails() async throws {
        let transport = StubTransport(
            scripted: loadReplies(detailBody) + [
                .json(.ok, voteResult("yes")), .failure(URLError(.notConnectedToInternet)),
            ])
        let model = ProposalDetailModel(
            id: "p", cabalID: "c", repository: repository(transport), hints: FakeHintStream())
        await model.load()
        let voted = await model.vote("yes")
        XCTAssertTrue(voted)
        XCTAssertNotNil(model.errorMessage)
    }
}

extension ProposalsRepositoryMappingTests {
    @MainActor
    func testChangingAVoteShowsTheNewBallotAndLocksTheScreenUntilTheReloadReturns() async throws {
        let before = detailBody.replacingOccurrences(of: #""my_ballot":null"#, with: #""my_ballot":"yes""#)
        let after = detailBody.replacingOccurrences(of: #""my_ballot":null"#, with: #""my_ballot":"no""#)
        let transport = StubTransport(scripted: loadReplies(before) + [.gate, .gate] + loadReplies(after))
        let model = ProposalDetailModel(
            id: "p", cabalID: "c", repository: repository(transport), hints: FakeHintStream())
        await model.load()
        XCTAssertEqual(model.summary?.myBallot, "yes")
        let voting = Task { await model.vote("no") }
        let posted = await eventArrives(within: 30) { await transport.waitForRequests(4) }
        XCTAssertTrue(posted)
        XCTAssertEqual(model.summary?.myBallot, "no")
        XCTAssertTrue(model.isVoting)
        await transport.releaseGate(.json(.ok, voteResult("no")))
        let reloading = await eventArrives(within: 30) { await transport.waitForRequests(5) }
        XCTAssertTrue(reloading)
        XCTAssertEqual(model.summary?.myBallot, "no")
        XCTAssertTrue(model.isVoting)
        await transport.releaseGate(.json(.ok, after))
        let voted = await voting.value
        XCTAssertTrue(voted)
        XCTAssertFalse(model.isVoting)
        XCTAssertEqual(model.summary?.myBallot, "no")
    }

    @MainActor
    func testRetryShowsTheTradeRunningWhileTheReloadStillReturnsTheFailedSwap() async throws {
        let failed = try failedSwapJSON(swapID: "swap-1")
        let transport = StubTransport(
            scripted: loadReplies(failed) + [.json(.accepted, #"{"swap_id":"swap-1","status":"retry_requested"}"#)]
                + loadReplies(failed))
        let model = ProposalDetailModel(
            id: "proposal-1", cabalID: "cabal-1", repository: repository(transport), hints: FakeHintStream())
        await model.load()
        XCTAssertEqual(model.retryableSwapID, "swap-1")
        XCTAssertEqual(model.summary?.swap?.status, "failed")
        await model.retry()
        XCTAssertTrue(model.didRetry)
        XCTAssertNil(model.retryableSwapID)
        XCTAssertEqual(model.summary?.status, .passed)
        XCTAssertNil(model.summary?.swap)
        XCTAssertEqual(model.value?.summary.swap?.id, "swap-1")
    }

    @MainActor
    func testANewSwapEndsTheRetryingState() async throws {
        let failed = try failedSwapJSON(swapID: "swap-1")
        let transport = StubTransport(
            scripted: loadReplies(failed) + [.json(.accepted, #"{"swap_id":"swap-1","status":"retry_requested"}"#)]
                + loadReplies(failed) + loadReplies(try failedSwapJSON(swapID: "swap-2")))
        let model = ProposalDetailModel(
            id: "proposal-1", cabalID: "cabal-1", repository: repository(transport), hints: FakeHintStream())
        await model.load()
        await model.retry()
        XCTAssertNil(model.retryableSwapID)
        await model.load()
        XCTAssertEqual(model.summary?.swap?.id, "swap-2")
        XCTAssertEqual(model.summary?.swap?.status, "failed")
        XCTAssertEqual(model.retryableSwapID, "swap-2")
    }

    @MainActor
    func testAnotherStatusEndsTheRetryingState() async throws {
        let failed = try failedSwapJSON(swapID: "swap-1")
        let voided = failed.replacingOccurrences(of: #""status":"passed""#, with: #""status":"voided""#)
        let transport = StubTransport(
            scripted: loadReplies(failed) + [.json(.accepted, #"{"swap_id":"swap-1","status":"retry_requested"}"#)]
                + loadReplies(failed) + loadReplies(voided))
        let model = ProposalDetailModel(
            id: "proposal-1", cabalID: "cabal-1", repository: repository(transport), hints: FakeHintStream())
        await model.load()
        await model.retry()
        await model.load()
        XCTAssertEqual(model.summary?.status, .voided)
        XCTAssertEqual(model.summary?.swap?.id, "swap-1")
    }

    @MainActor
    func testAFailedRetryKeepsTheRetryButton() async throws {
        let failed = try failedSwapJSON(swapID: "swap-1")
        let transport = StubTransport(
            scripted: loadReplies(failed) + [.failure(URLError(.notConnectedToInternet))])
        let model = ProposalDetailModel(
            id: "proposal-1", cabalID: "cabal-1", repository: repository(transport), hints: FakeHintStream())
        await model.load()
        await model.retry()
        XCTAssertFalse(model.didRetry)
        XCTAssertEqual(model.retryableSwapID, "swap-1")
        XCTAssertEqual(model.summary?.swap?.status, "failed")
    }

    @MainActor
    func testCardContextReadsMembersAndEveryAssetTogether() async throws {
        let transport = StubTransport(scripted: [.gate, .gate, .gate])
        let context = ProposalCardContext(cabalID: "c", repository: repository(transport))
        let loading = Task { await context.load(for: [proposal(symbol: "AAPLx"), proposal(symbol: "TSLAx")]) }
        let issued = await eventArrives(within: 30) { await transport.waitForRequests(3) }
        XCTAssertTrue(issued)
        XCTAssertFalse(context.hasLoaded)
        XCTAssertTrue(context.members.isEmpty)
        XCTAssertTrue(context.assets.isEmpty)
        for path in await transport.sent.compactMap(\.path) {
            let reply = path.hasSuffix("TSLAx") ? assetBody.replacingOccurrences(of: "AAPLx", with: "TSLAx") : assetBody
            await transport.releaseGate(.json(.ok, path.contains("/v1/cabals/") ? cabalBody : reply))
        }
        await loading.value
        XCTAssertTrue(context.hasLoaded)
        XCTAssertEqual(context.members.map(\.name), ["Jordan"])
        XCTAssertEqual(Set(context.assets.keys), ["AAPLx", "TSLAx"])
    }

    @MainActor
    func testCardContextFetchesOnlyWhatIsMissing() async throws {
        let transport = PathRoutedTransport([
            "/v1/cabals/c": [.json(.ok, cabalBody)],
            "/v1/assets/AAPLx": [.json(.ok, assetBody)],
            "/v1/assets/TSLAx": [.json(.ok, assetBody.replacingOccurrences(of: "AAPLx", with: "TSLAx"))],
        ])
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token"), transport: transport)
        let context = ProposalCardContext(cabalID: "c", repository: ProposalsRepository(api: api))
        await context.load(for: [proposal(symbol: "AAPLx")])
        await context.load(for: [proposal(symbol: "AAPLx"), proposal(symbol: "TSLAx")])
        let paths = await transport.sent.compactMap(\.path)
        XCTAssertEqual(paths.sorted(), ["/v1/assets/AAPLx", "/v1/assets/TSLAx", "/v1/cabals/c"])
        XCTAssertEqual(Set(context.assets.keys), ["AAPLx", "TSLAx"])
    }

    @MainActor
    func testCardContextFinishesItsFirstPassWhenEveryReadFails() async throws {
        let transport = StubTransport(.failure(URLError(.notConnectedToInternet)))
        let context = ProposalCardContext(cabalID: "c", repository: repository(transport))
        await context.load(for: [proposal(symbol: "AAPLx")])
        XCTAssertTrue(context.hasLoaded)
        XCTAssertTrue(context.members.isEmpty)
        XCTAssertTrue(context.assets.isEmpty)
    }

    private func proposal(symbol: String) -> ProposalSummary {
        var proposal = Components.Schemas.Proposal.sample()
        proposal.symbol = symbol
        return ProposalSummary(proposal)
    }

    private func failedSwapJSON(swapID: String) throws -> String {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        var detail = Components.Schemas.ProposalDetail.failedSwap(retryable: true)
        detail.swap?.swapId = swapID
        return String(decoding: try encoder.encode(detail), as: UTF8.self)
    }
}
