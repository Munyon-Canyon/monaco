import MonacoAPI
import MonacoCore
import XCTest

final class CabalPotSummaryTests: XCTestCase {
    private typealias Pot = Components.Schemas.CabalPot

    func testAnInvestedPotShowsItsFiguresHoldingsAndMix() {
        let summary = CabalPotSummary(Pot.sampleInvested)

        XCTAssertEqual(summary.state, .invested)
        XCTAssertEqual(summary.potValue, "$1,000.00")
        XCTAssertEqual(summary.allTime, "+$0.73")
        XCTAssertEqual(summary.cash, "$750.00")
        XCTAssertEqual(summary.invested, "$250.00")
        XCTAssertEqual(summary.slice, .stake(value: "$380.15", ofPot: "38% of the pot", gain: "+$0.15"))
        let row = summary.holdings.first
        XCTAssertEqual(summary.holdings.count, 1)
        XCTAssertEqual(row?.symbol, "GOOGLx")
        XCTAssertEqual(row?.ticker, "GOOGL")
        XCTAssertEqual(row?.name, "Alphabet")
        XCTAssertEqual(row?.detail, "0.73 shares · $342.47")
        XCTAssertEqual(row?.value, "$250.00")
        XCTAssertEqual(row?.gain, "+$0.73")
        XCTAssertEqual(summary.legend.map(\.label), ["GOOGL", "Cash"])
        XCTAssertEqual(summary.legend.map(\.percent), ["25%", "75%"])
        XCTAssertEqual(summary.legend.map(\.isCash), [false, true])
    }

    func testAPotWithNoHoldingsIsCashOnly() {
        let summary = CabalPotSummary(Pot.sampleCashOnly)

        XCTAssertEqual(summary.state, .cashOnly)
        XCTAssertEqual(summary.invested, "$0.00")
        XCTAssertTrue(summary.holdings.isEmpty)
        XCTAssertEqual(summary.legend.map(\.percent), ["100%"])
    }

    func testAnEmptyPotIsZeroAndAMemberWithNoSharesHasNoStake() {
        let summary = CabalPotSummary(Pot.sampleZero)

        XCTAssertEqual(summary.state, .zero)
        XCTAssertEqual(summary.potValue, "$0.00")
        XCTAssertEqual(summary.allTime, "$0.00")
        XCTAssertEqual(summary.slice, CabalPotSummary.Slice.none)
    }

    func testANonMemberHasNoSlice() {
        let summary = CabalPotSummary(Pot.sampleOutsider)

        XCTAssertNil(summary.slice)
        XCTAssertEqual(summary.state, .invested)
    }

    func testCashTakesTheServersCashWeightAndATinyHoldingReadsUnderOnePercent() {
        let summary = CabalPotSummary(
            Pot.sample(
                potValueMicros: 1_000_000_000, cashMicros: 990_000_000, cashWeightBps: 9950, pnlMicros: 0,
                holdings: [holding("TSLAx", units: "0.0100", weightBps: 50, pnlMicros: 0)], me: nil))

        XCTAssertEqual(summary.legend.map(\.percent), ["<1%", "100%"])
        XCTAssertEqual(summary.legend.map(\.basisPoints), [50, 9950])
    }

    func testPercentRoundsHalfUp() {
        let summary = CabalPotSummary(
            Pot.sample(
                potValueMicros: 1_000_000_000, cashMicros: 0, cashWeightBps: 0, pnlMicros: 0,
                holdings: [
                    holding("AAPLx", units: "1.0000", weightBps: 2550, pnlMicros: 0),
                    holding("NVDAx", units: "2.0000", weightBps: 7449, pnlMicros: 0),
                    holding("MSFTx", units: "10.0000", weightBps: 1, pnlMicros: 0),
                ],
                me: nil))

        XCTAssertEqual(summary.legend.map(\.percent), ["26%", "74%", "<1%", "0%"])
        XCTAssertEqual(
            summary.holdings.map(\.detail), ["1 share · $10.00", "2 shares · $10.00", "10 shares · $10.00"])
    }

    func testALossReadsWithTheTypographicMinus() {
        let summary = CabalPotSummary(
            Pot.sample(
                potValueMicros: 90_000_000, cashMicros: 0, cashWeightBps: 0, pnlMicros: -10_000_000,
                holdings: [holding("AAPLx", units: "0.7300", weightBps: 10000, pnlMicros: -10_000_000)],
                me: .init(
                    shareUnits: 100, valueMicros: 90_000_000, sliceBps: 10000, netContributedMicros: 100_000_000,
                    pnlMicros: -10_000_000)))

        XCTAssertEqual(summary.allTime, "\u{2212}$10.00")
        XCTAssertEqual(summary.holdings.first?.gain, "\u{2212}$10.00")
        XCTAssertEqual(summary.holdings.first?.detail, "0.73 shares · $10.00")
        XCTAssertEqual(summary.slice, .stake(value: "$90.00", ofPot: "100% of the pot", gain: "\u{2212}$10.00"))
    }

    private func holding(_ symbol: String, units: String, weightBps: Int32, pnlMicros: Int64)
        -> Components.Schemas.CabalHolding
    {
        .init(
            symbol: symbol, displayName: symbol, kind: .equity, units: units, tokenAmount: 100_000_000,
            priceMicros: 10_000_000, valueMicros: 10_000_000, weightBps: weightBps,
            costBasisMicros: 10_000_000 - pnlMicros, pnlMicros: pnlMicros)
    }
}

extension CabalPotSummaryTests {
    func testEverySegmentHasOneSwatchInHoldingsOrderWithCashLast() {
        let legend = CabalPotSummary(Components.Schemas.CabalPot.sampleInvested).legend

        XCTAssertEqual(legend.map(\.swatch), [.stock(step: 0), .cash])
        XCTAssertEqual(CabalPotSummary.Swatch.forStock(at: 7), .stock(step: CabalPotSummary.Swatch.steps - 1))
        XCTAssertEqual(CabalPotSummary.Swatch.forStock(at: -1), .stock(step: 0))
    }

    func testALogoIsAttachedBySymbolAndAMissingOneLeavesTheTileFallback() throws {
        let summary = CabalPotSummary(Components.Schemas.CabalPot.sampleInvested)
        XCTAssertNil(summary.holdings.first?.logoURL)

        let url = try XCTUnwrap(URL(string: "https://cdn.example.com/GOOGLx.png"))
        XCTAssertEqual(summary.withLogos(["GOOGLx": url]).holdings.first?.logoURL, url)
        XCTAssertNil(summary.withLogos(["AAPLx": url]).holdings.first?.logoURL)
        XCTAssertEqual(summary.withLogos([:]).holdings, summary.holdings)
    }
}
