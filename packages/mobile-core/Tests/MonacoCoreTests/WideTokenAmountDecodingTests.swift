import Foundation
import MonacoAPI
import XCTest

@testable import MonacoCore

final class WideTokenAmountDecodingTests: XCTestCase {
    private static let twentyFive = Decimal(string: "25000000000000000000") ?? 0
    private static let fourThousand = Decimal(string: "4000000000000000000000") ?? 0

    private func decode<T: Decodable>(_ type: T.Type, _ json: String) throws -> T {
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        return try decoder.decode(type, from: Data(json.utf8))
    }

    private func proposalJSON(_ amounts: String) -> String {
        """
        {"id":"01890a5d-ac96-774b-bcce-b302099a8057","cabal_id":"01890a5d-ac96-774b-bcce-b302099a8058",
        "cabal_name":"Friday Night Lights","proposer_id":"01890a5d-ac96-774b-bcce-b302099a8059","kind":"sell",
        "rule_change":null,"symbol":"AAPLx","usdc_micros":null,\(amounts),"thesis":null,"status":"open",
        "status_reason":null,"status_message":null,"expires_at":"2026-10-04T15:00:00Z",
        "created_at":"2026-10-03T15:00:00Z","tally":{"yes":1,"no":0,"voters":2,"needed":2},"my_ballot":null,
        "can_vote":true,"reply_count":0}
        """
    }

    func testAProposalPastInt64ReadsItsStringFields() throws {
        let json = proposalJSON(
            #""token_amount":null,"token_amount_str":"25000000000000000000","quote_out_amount":0,"#
                + #""quote_out_amount_str":"4000000000000000000000""#)
        let summary = ProposalSummary(try decode(Components.Schemas.Proposal.self, json))
        XCTAssertEqual(summary.tokenAmount, Self.twentyFive)
        XCTAssertEqual(summary.quoteOutAmount, Self.fourThousand)
    }

    func testAProposalFromAnOlderServerFallsBackToTheIntegerFields() throws {
        let json = proposalJSON(#""token_amount":150000000,"quote_out_amount":105000000"#)
        let summary = ProposalSummary(try decode(Components.Schemas.Proposal.self, json))
        XCTAssertEqual(summary.tokenAmount, 150_000_000)
        XCTAssertEqual(summary.quoteOutAmount, 105_000_000)
    }

    func testAProposalThatFitsReadsTheSameFromBothFields() throws {
        let json = proposalJSON(
            #""token_amount":9223372036854775807,"token_amount_str":"9223372036854775807","#
                + #""quote_out_amount":7,"quote_out_amount_str":"7""#)
        let summary = ProposalSummary(try decode(Components.Schemas.Proposal.self, json))
        XCTAssertEqual(summary.tokenAmount, Decimal(Int64.max))
        XCTAssertEqual(summary.quoteOutAmount, 7)
    }

    func testAProposalWithoutAnAmountHasNone() throws {
        let json = proposalJSON(#""token_amount":null,"quote_out_amount":null"#)
        let summary = ProposalSummary(try decode(Components.Schemas.Proposal.self, json))
        XCTAssertNil(summary.tokenAmount)
        XCTAssertEqual(summary.quoteOutAmount, 0)
    }

    func testAPendingVotePastInt64ReadsItsStringField() throws {
        let json = """
            {"proposal_id":"01890a5d-ac96-774b-bcce-b302099a8057","cabal_id":"01890a5d-ac96-774b-bcce-b302099a8058",
            "cabal_name":"Friday Night Lights","kind":"sell","rule_change":null,"symbol":"AAPLx","usdc_micros":null,
            "token_amount":null,"token_amount_str":"25000000000000000000","asset_name":"Apple","asset_logo_url":null,
            "proposer":{"user_id":"01890a5d-ac96-774b-bcce-b302099a8059","handle":"mia","display_name":"Mia",
            "photo_url":null},"tally":{"yes":4,"no":1,"voters":8,"needed":5},"voted_count":5,"reply_count":6,
            "created_at":"2026-10-04T09:00:00Z","expires_at":"2026-10-04T15:00:00Z"}
            """
        let vote = try decode(Components.Schemas.PendingVote.self, json)
        XCTAssertEqual(HomePendingVote(vote).tokenAmount, Self.twentyFive)
    }

    func testATradePreviewPastInt64ReadsItsStringField() throws {
        let json = """
            {"quote_out_amount":null,"quote_out_amount_str":"4000000000000000000000","advisory_code":null,
            "advisory_message":null,"pot_value_micros":100000000}
            """
        let preview = ProposePreview(try decode(Components.Schemas.TradePreview.self, json))
        XCTAssertEqual(preview.quoteOutAmount, Self.fourThousand)
        XCTAssertNil(preview.quoteOutMicros)
    }

    func testATradePreviewFromAnOlderServerKeepsItsInteger() throws {
        let json = """
            {"quote_out_amount":21000000,"advisory_code":null,"advisory_message":null,"pot_value_micros":100000000}
            """
        let preview = ProposePreview(try decode(Components.Schemas.TradePreview.self, json))
        XCTAssertEqual(preview.quoteOutAmount, 21_000_000)
        XCTAssertEqual(preview.quoteOutMicros, 21_000_000)
    }

    func testALivePriceComparisonPastInt64ReadsItsStringField() throws {
        let json = """
            {"quote_out_amount":0,"quote_out_amount_str":"4000000000000000000000","quoted_at":"2026-10-10T12:00:00Z"}
            """
        let live = LivePriceComparison(try decode(Components.Schemas.ProposalPriceComparison.self, json))
        XCTAssertEqual(live.quoteOutAmount, Self.fourThousand)
    }

    func testAnActivityRowPastInt64ReadsItsStringField() throws {
        let json = """
            {"id":"01890a5d-ac96-774b-bcce-b302099a8060","kind":"buy","status":"confirmed",
            "asset":{"symbol":"AAPLx","name":"Apple"},"usdc_micros":25000000,"units":null,
            "units_str":"25000000000000000000","price_micros":1,"actor":null,"tx_signature":null,
            "occurred_at":"2026-10-03T15:00:00Z"}
            """
        let activity = try decode(Components.Schemas.CabalActivity.self, json)
        XCTAssertEqual(activity.unitsValue, Self.twentyFive)
        let trade = try XCTUnwrap(HoldingTrade(activity, symbol: "AAPLx", decimals: 18))
        XCTAssertEqual(trade.unitsText, "25.0000")
    }

    func testAHoldingPastInt64KeepsItsExactTokenAmountAndStillCannotBeSoldWhole() throws {
        let json = """
            {"symbol":"AAPLx","display_name":"Apple","kind":"equity","units":"25.0000","token_amount":0,
            "token_amount_str":"25000000000000000000","price_micros":231400000,"value_micros":5785000000,
            "weight_bps":5000,"cost_basis_micros":4000000000,"pnl_micros":1785000000,"avg_entry_micros":"160000000",
            "first_bought_at":"2026-10-01T15:00:00Z"}
            """
        let holding = ProposeHolding(try decode(Components.Schemas.CabalHolding.self, json))
        XCTAssertEqual(holding.tokenAmount, Self.twentyFive)
        XCTAssertTrue(holding.overflows)
        XCTAssertTrue(holding.exceedsSettleLimit(forMicros: 5_785_000_000))
        XCTAssertEqual(holding.tokenAmount(forMicros: 2_314_000_000), Decimal(string: "10000000000000000000"))
    }

    func testAFeedItemPastInt64ReadsItsStringFields() throws {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        let sample = try XCTUnwrap(Components.Schemas.FeedItem.samples.first)
        var fields = try XCTUnwrap(
            JSONSerialization.jsonObject(with: try encoder.encode(sample)) as? [String: Any])
        fields["token_amount"] = NSNull()
        fields["token_amount_str"] = "25000000000000000000"
        fields["filled_token_amount"] = NSNull()
        fields["filled_token_amount_str"] = "25000000000000000000"
        let item = try decode(
            Components.Schemas.FeedItem.self,
            String(decoding: try JSONSerialization.data(withJSONObject: fields), as: UTF8.self))
        XCTAssertEqual(item.tokenAmountValue, Self.twentyFive)
        XCTAssertEqual(item.filledTokenAmountValue, Self.twentyFive)
    }

    func testTheSellRequestSendsTheIntegerWhenItFitsAndTheStringWhenItDoesNot() throws {
        let fits = ProposalDraft.sell(symbol: "AAPLx", tokenAmount: 150_000_000, thesis: "")
        XCTAssertEqual(TokenAmount.int64(fits.amount), 150_000_000)
        let wide = ProposalDraft.sell(symbol: "AAPLx", tokenAmount: Self.twentyFive, thesis: "")
        XCTAssertNil(TokenAmount.int64(wide.amount))
        XCTAssertEqual(TokenAmount.text(wide.amount), "25000000000000000000")
    }
}
