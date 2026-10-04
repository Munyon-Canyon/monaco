import Foundation
import MonacoAPI
import MonacoCore
import XCTest

final class ProposalsRepositoryTests: XCTestCase {
    private let now = Date(timeIntervalSince1970: 1_790_000_000)
    private let kai = Components.Schemas.ProposalDetail.sampleVoterIDs[0]
    private let jordan = Components.Schemas.ProposalDetail.sampleVoterIDs[1]

    func testTheOpenTabAsksForOpenProposals() async throws {
        let open = Components.Schemas.ProposalDetail.sample(id: "p-open", now: now)
        let closed = Components.Schemas.ProposalDetail.sample(id: "p-closed", status: .expired, now: now)
        let server = ProposalsPreviewServer(proposals: [open, closed])
        let repository = ProposalsRepository.preview(server)

        let openPage = try await repository.list(cabalID: open.cabalId, filter: .open, cursor: nil)
        let closedPage = try await repository.list(cabalID: open.cabalId, filter: .closed, cursor: nil)

        XCTAssertEqual(openPage.items.map(\.id), ["p-open"])
        XCTAssertEqual(closedPage.items.map(\.id), ["p-closed"])
        let requests = await server.requests
        XCTAssertEqual(
            requests,
            [
                "GET /v1/cabals/\(open.cabalId)/proposals?filter=open",
                "GET /v1/cabals/\(open.cabalId)/proposals?filter=closed",
            ])
    }

    func testDetailMapsTheTradeAndTheBlockedMessage() async throws {
        let blocked = Components.Schemas.ProposalDetail.sample(
            id: "p-blocked", status: .executionBlocked, statusMessage: "The pot is short.", now: now)
        let failed = Components.Schemas.ProposalDetail.sample(
            id: "p-failed", status: .passed, swap: .failed(retryable: true), now: now)
        let repository = ProposalsRepository.preview(ProposalsPreviewServer(proposals: [blocked, failed]))

        let blockedDetail = try await repository.detail(id: "p-blocked")
        XCTAssertEqual(blockedDetail.proposal.status, .executionBlocked)
        XCTAssertEqual(blockedDetail.steps.titles.last, "Couldn't buy")
        XCTAssertEqual(blockedDetail.steps.failureMessage, "The pot is short.")
        XCTAssertNil(blockedDetail.trade)

        let failedDetail = try await repository.detail(id: "p-failed")
        XCTAssertEqual(failedDetail.trade?.state, .failed)
        XCTAssertEqual(failedDetail.trade?.retryable, true)
        XCTAssertEqual(failedDetail.trade?.swapID, "01890a5d-ac96-774b-bcce-b302099a8063")
        XCTAssertEqual(
            failedDetail.steps.failureMessage, "The price moved too far before the trade went through.")
    }

    func testAVoteSendsTheChoiceWithAnIdempotencyKey() async throws {
        let proposal = Components.Schemas.ProposalDetail.sample(now: now)
        let server = ProposalsPreviewServer(proposals: [proposal])
        let repository = ProposalsRepository.preview(server)

        try await repository.vote(id: proposal.id, choice: .no, submission: IdempotentSubmission())

        let detail = try await repository.detail(id: proposal.id)
        XCTAssertEqual(detail.proposal.myBallot, .no)
        XCTAssertEqual(detail.proposal.tally.no, 1)
        let count = await server.count("POST /v1/proposals/\(proposal.id)/votes")
        XCTAssertEqual(count, 1)
    }

    func testPendingVotesKeepTheServerOrder() async throws {
        var later = Components.Schemas.ProposalDetail.sample(id: "p-later", now: now)
        later.expiresAt = now.addingTimeInterval(20 * 3600)
        var sooner = Components.Schemas.ProposalDetail.sample(id: "p-sooner", now: now)
        sooner.expiresAt = now.addingTimeInterval(3600)
        let voted = Components.Schemas.ProposalDetail.sample(id: "p-voted", ballots: [kai: .yes], now: now)
        let repository = ProposalsRepository.preview(ProposalsPreviewServer(proposals: [later, voted, sooner]))

        let pending = try await repository.pendingVotes()

        XCTAssertEqual(pending.map(\.id), ["p-sooner", "p-later"])
    }

    func testLookupsNameTheProposerAndFallBackForAnUnknownAsset() async throws {
        let known = Components.Schemas.ProposalDetail.sample(id: "p-known", now: now)
        let unknown = Components.Schemas.ProposalDetail.sample(id: "p-unknown", symbol: "ZZZZx", now: now)
        let server = ProposalsPreviewServer(proposals: [known, unknown])
        let repository = ProposalsRepository.preview(server)
        let proposals = [try await repository.detail(id: "p-known"), try await repository.detail(id: "p-unknown")]

        let lookups = try await repository.lookups(for: proposals.map(\.proposal))

        let card = lookups.card(proposals[0].proposal)
        XCTAssertEqual(card.proposer.name, "Jordan")
        XCTAssertEqual(card.proposer.userID, jordan)
        XCTAssertEqual(card.asset.ticker, "GOOGL")
        XCTAssertEqual(card.asset.name, "Alphabet")
        XCTAssertEqual(card.ballot, .ask)
        XCTAssertEqual(lookups.card(proposals[1].proposal).asset.ticker, "ZZZZ")
        let cabalReads = await server.count("GET /v1/cabals/")
        XCTAssertEqual(cabalReads, 1)
    }

    func testALookupFailureOtherThanAnUnknownAssetFailsTheLoad() async throws {
        let server = ProposalsPreviewServer(proposals: [.sample(now: now)])
        let repository = ProposalsRepository.preview(server)
        let proposal = try await repository.detail(id: Components.Schemas.ProposalDetail.sample(now: now).id)
        await server.setFailing(true)

        do {
            _ = try await repository.lookups(for: [proposal.proposal])
            XCTFail("expected the lookup to fail")
        } catch {
            XCTAssertEqual(ToastCopy.message(for: APIError(error)), "Monaco is busy. Try again.")
        }
    }
}
