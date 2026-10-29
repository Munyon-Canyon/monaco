import Foundation
import MonacoAPI
import XCTest

@testable import MonacoCore

final class RuleProposalDecodeTests: XCTestCase {
    private let now = Date(timeIntervalSince1970: 1_789_682_400)

    func testProposalDecodesARuleProposalWithNullTradeFields() async throws {
        let json = """
            {"id":"proposal-rule","cabal_id":"cabal-1","cabal_name":"Friday Night Lights","proposer_id":"user-1",
             "kind":"rule","rule_change":{"rule":"show_sides","from":false,"to":true,"user":null},"symbol":null,
             "usdc_micros":null,"token_amount":null,"quote_out_amount":null,"thesis":null,"status":"open",
             "status_reason":null,"status_message":null,"expires_at":"2026-09-18T22:00:00Z",
             "created_at":"2026-09-17T22:00:00Z","tally":{"yes":3,"no":1,"voters":8,"needed":5},
             "my_ballot":null,"can_vote":true}
            """

        let proposal = try monacoISO8601JSONDecoder().decode(
            Components.Schemas.Proposal.self, from: Data(json.utf8))

        XCTAssertEqual(proposal.kind, .rule)
        XCTAssertNil(proposal.symbol)
        XCTAssertNil(proposal.usdcMicros)
        XCTAssertNil(proposal.quoteOutAmount)
        XCTAssertEqual(proposal.ruleChange?.rule, .showSides)
        XCTAssertNil(proposal.ruleChange?.user)
    }

    func testPendingVoteDecodesARuleProposalWithNullTradeFields() async throws {
        let json = """
            {"proposal_id":"proposal-rule","cabal_id":"cabal-1","cabal_name":"Friday Night Lights","kind":"rule",
             "rule_change":{"rule":"voter","from":false,"to":true,
              "user":{"user_id":"user-kai","handle":"kai","display_name":"Kai","photo_url":null}},
             "symbol":null,"usdc_micros":null,"token_amount":null,"asset_name":null,"asset_logo_url":null,
             "proposer":{"user_id":"user-mia","handle":"mia","display_name":"Mia","photo_url":null},
             "tally":{"yes":3,"no":1,"voters":8,"needed":5},"voted_count":4,"reply_count":0,
             "created_at":"2026-09-17T22:00:00Z","expires_at":"2026-09-18T22:00:00Z"}
            """

        let vote = try monacoISO8601JSONDecoder().decode(
            Components.Schemas.PendingVote.self, from: Data(json.utf8))

        XCTAssertEqual(vote.kind, .rule)
        XCTAssertNil(vote.symbol)
        XCTAssertNil(vote.assetName)
        XCTAssertEqual(vote.ruleChange?.rule, .voter)
        XCTAssertEqual(vote.ruleChange?.user?.handle, "kai")
    }

    func testTheRuleSamplesRoundTripThroughTheWireFormat() async throws {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        let decoder = monacoISO8601JSONDecoder()

        let proposal = try decoder.decode(
            Components.Schemas.Proposal.self,
            from: encoder.encode(Components.Schemas.Proposal.ruleSample(now: now)))
        let detail = try decoder.decode(
            Components.Schemas.ProposalDetail.self,
            from: encoder.encode(Components.Schemas.ProposalDetail.ruleSample(now: now)))
        let vote = try decoder.decode(
            Components.Schemas.PendingVote.self,
            from: encoder.encode(Components.Schemas.PendingVote.ruleSample(now: now)))

        XCTAssertEqual(proposal.ruleChange?.rule, .showSides)
        XCTAssertEqual(detail.ruleChange?.rule, .showSides)
        XCTAssertNil(detail.symbol)
        XCTAssertEqual(vote.votedCount, 4)
        XCTAssertEqual(vote.tally.voters, 8)
    }

    func testTheRepositoryMapsARuleProposalWithoutATradeSymbol() async throws {
        let summary = ProposalSummary(.ruleSample(now: now))

        XCTAssertEqual(summary.kind, "rule")
        XCTAssertEqual(summary.symbol, "")
    }
}
