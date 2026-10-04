import Foundation
import MonacoAPI
import MonacoCore
import XCTest

final class ProposalCardTests: XCTestCase {
    private let now = Date(timeIntervalSince1970: 1_790_000_000)
    private let kai = Components.Schemas.ProposalDetail.sampleVoterIDs[0]

    func testABuyShowsDollarsAndExpectsSharesAtAPrice() async throws {
        let card = try await card(.sample(now: now))
        XCTAssertEqual(card.amount, "$250.00")
        XCTAssertEqual(card.asset.expected(of: card.proposal), "about 0.7319 shares at $341.57")
        XCTAssertEqual(card.closes(now: now), "Closes in 23h")
        XCTAssertNil(card.chip)
        XCTAssertEqual(card.tracker, "0 of 3 voted · 2 yes to pass")
    }

    func testASellShowsSharesOrTokensAndExpectsDollars() async throws {
        let sell = try await card(.sample(kind: .sell, now: now))
        XCTAssertEqual(sell.amount, "0.6017 shares")
        XCTAssertEqual(sell.asset.expected(of: sell.proposal), "about $139.00")

        let preIpo = try await card(
            .sample(kind: .sell, symbol: "tSpaceX", now: now),
            assets: [.proposalSample(symbol: "tSpaceX", kind: .preIpo)])
        XCTAssertEqual(preIpo.amount, "0.6017 tokens")
    }

    func testAMillionDollarBuyStillPricesTheShare() async throws {
        var big = Components.Schemas.ProposalDetail.sample(now: now)
        big.usdcMicros = 1_000_000_000_000
        big.quoteOutAmount = 292_765_000_000
        let card = try await card(big)
        XCTAssertEqual(card.amount, "$1,000,000.00")
        XCTAssertEqual(card.asset.expected(of: card.proposal), "about 2,927.65 shares at $341.57")
    }

    func testTheBallotFollowsCanVoteMyBallotAndStatus() async throws {
        let ask = try await card(.sample(now: now))
        XCTAssertEqual(ask.ballot, .ask)
        XCTAssertTrue(ask.awaitsViewer)

        let voted = try await card(.sample(ballots: [kai: .yes], now: now))
        XCTAssertEqual(voted.ballot, .voted(.yes))
        XCTAssertEqual(voted.tracker, "1 of 3 voted · 2 yes to pass")

        let nonVoter = try await card(.sample(canVote: false, now: now))
        XCTAssertEqual(nonVoter.ballot, .none)

        let closed = try await card(.sample(status: .executed, ballots: [kai: .yes], now: now))
        XCTAssertEqual(closed.ballot, .none)
        XCTAssertEqual(closed.chip, "Bought")
        XCTAssertNil(closed.closes(now: now))
    }

    func testAFailedSwapShowsCouldNotBuyOnTheCard() async throws {
        let card = try await card(.sample(status: .passed, swap: .failed(retryable: false), now: now))
        XCTAssertEqual(card.chip, "Couldn't buy")
    }

    private func card(
        _ detail: Components.Schemas.ProposalDetail,
        assets: [Components.Schemas.AssetDetail] = [.proposalSample()]
    ) async throws -> ProposalCard {
        let repository = ProposalsRepository.preview(ProposalsPreviewServer(proposals: [detail], assets: assets))
        let loaded = try await repository.detail(id: detail.id)
        let lookups = try await repository.lookups(for: [loaded.proposal])
        return lookups.card(loaded.proposal, canVote: loaded.canVote, swap: loaded.trade?.state)
    }
}
