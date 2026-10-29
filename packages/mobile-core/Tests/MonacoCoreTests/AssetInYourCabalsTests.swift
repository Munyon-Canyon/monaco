import MonacoAPI
import MonacoCore
import XCTest

final class AssetInYourCabalsTests: XCTestCase {
    private let now = Date(timeIntervalSince1970: 1_800_000_000)

    func testTheHeldRowReadsLikeTheDesignFrame() throws {
        let facts = AssetHeldRowFacts(try heldRow(pnl: 1_026_666_667, cost: 7_333_333_333))

        XCTAssertEqual(facts.title, "AC Capital")
        XCTAssertEqual(facts.subtitle, "38 sh · 42% of treasury")
        XCTAssertEqual(facts.valueText, "$8,360.00")
        XCTAssertEqual(facts.returnText, "+14.0%")
    }

    func testAFractionalWeightIsFlooredAndUnitsKeepTheirDecimals() throws {
        let facts = AssetHeldRowFacts(try heldRow(units: "2.5000", weightBps: 4299, pnl: 0, cost: 1))

        XCTAssertEqual(facts.subtitle, "2.5 sh · 42% of treasury")
    }

    func testANegativeReturnUsesTheTypographicMinus() throws {
        let facts = AssetHeldRowFacts(try heldRow(pnl: -500_000_000, cost: 1_000_000_000))

        XCTAssertEqual(facts.returnText, "\u{2212}50.0%")
    }

    func testAZeroCostBasisHasNoReturn() throws {
        XCTAssertNil(AssetHeldRowFacts(try heldRow(pnl: 5, cost: 0)).returnText)
    }

    func testTheOpenProposalRowReadsLikeTheDesignFrame() throws {
        let facts = try XCTUnwrap(AssetOpenProposalRowFacts(proposal(), now: now))

        XCTAssertEqual(facts.title, "AC Capital \u{00B7} Buy $400")
        XCTAssertEqual(facts.subtitle, "5 of 8 voted \u{00B7} 6 replies \u{00B7} ends in 4h")
        XCTAssertEqual(facts.trailing, "\u{2713} 4 \u{00B7} \u{2715} 1")
        XCTAssertEqual(facts.proposalID, "proposal-1")
    }

    func testAnAmountWithCentsKeepsThem() throws {
        var item = proposal()
        item.usdcMicros = 400_500_000

        XCTAssertEqual(AssetOpenProposalRowFacts(item, now: now)?.title, "AC Capital \u{00B7} Buy $400.50")
    }

    func testAFeedItemWithoutATallyIsNotAnOpenProposalRow() {
        var item = proposal()
        item.tally = nil

        XCTAssertNil(AssetOpenProposalRowFacts(item, now: now))
    }

    private func proposal() -> Components.Schemas.FeedItem {
        var item = Components.Schemas.FeedItem.samples[0]
        item.proposalId = "proposal-1"
        item.cabalName = "AC Capital"
        item.action = "buy"
        item.usdcMicros = 400_000_000
        item.commentCount = 6
        item.tally = .init(yes: 4, no: 1, voters: 8, needed: 5)
        item.expiresAt = now.addingTimeInterval(4 * 3600 + 30)
        return item
    }

    private func heldRow(
        units: String = "38.0000", weightBps: Int = 4200, pnl: Int64, cost: Int64
    ) throws -> Components.Schemas.HeldRow {
        let json = #"""
            {"cabal":{"id":"c-1","name":"AC Capital","picture_url":null},"logo_url":null,"holding":{"symbol":"NVDAx","display_name":"NVIDIA","kind":"equity","units":"\#(units)","token_amount":38000000,"price_micros":220000000,"value_micros":8360000000,"weight_bps":\#(weightBps),"cost_basis_micros":\#(cost),"pnl_micros":\#(pnl),"avg_entry_micros":"1","first_bought_at":null}}
            """#
        return try JSONDecoder().decode(Components.Schemas.HeldRow.self, from: Data(json.utf8))
    }
}
