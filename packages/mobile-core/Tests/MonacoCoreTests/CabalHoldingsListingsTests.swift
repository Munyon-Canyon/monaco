import MonacoAPI
import MonacoCore
import XCTest

final class CabalHoldingsListingsTests: XCTestCase {
    func testAStockHeldOnTwoChainsIsOneHoldingsRow() {
        let listings = [
            Components.Schemas.HoldingListing(
                listingId: "01928a6e-0000-7000-8000-000000000001", issuer: "xstocks", network: .solana,
                tokenAmount: "50000000", valueMicros: 171_235_000),
            Components.Schemas.HoldingListing(
                listingId: "01928a6e-0000-7000-8000-000000000002", issuer: "xstocks", network: .base,
                tokenAmount: "23000000000000000", valueMicros: 78_765_000),
        ]
        let holding = Components.Schemas.CabalHolding(
            listings: listings, symbol: "GOOGLx", displayName: "Alphabet", kind: .equity, units: "0.7300",
            tokenAmount: 73_000_000, priceMicros: 342_470_000, valueMicros: 250_000_000, weightBps: 2500,
            costBasisMicros: 249_270_000, pnlMicros: 730_000)
        let pot = Components.Schemas.CabalPot.sample(
            potValueMicros: 1_000_000_000, cashMicros: 750_000_000, cashWeightBps: 7500, pnlMicros: 730_000,
            holdings: [holding], me: nil)

        let summary = CabalPotSummary(pot)

        XCTAssertEqual(summary.holdings.count, 1)
        XCTAssertEqual(summary.holdings.first?.name, "Alphabet")
        XCTAssertEqual(summary.holdings.first?.detail, "0.73 shares · $342.47")
        XCTAssertEqual(summary.holdings.first?.value, "$250.00")
        XCTAssertEqual(summary.legend.map(\.label), ["GOOGL", "Cash"])
    }
}
