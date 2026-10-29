import MonacoAPI
import XCTest

@testable import MonacoCore

final class CabalTreasuryRowsTests: XCTestCase {
    private typealias Pot = Components.Schemas.CabalPot
    private typealias Holding = Components.Schemas.CabalHolding

    private func holding(symbol: String, units: String, value: Int64, cost: Int64, pnl: Int64) -> Holding {
        Holding(
            symbol: symbol, displayName: symbol, kind: .equity, units: units, tokenAmount: 0, priceMicros: 0,
            valueMicros: value, weightBps: 0, costBasisMicros: cost, pnlMicros: pnl)
    }

    private func pot(_ holdings: [Holding], shareMicros: Int64 = 7_000_000_000) -> Pot {
        var pot: Pot = .sampleACCapital
        pot.holdings = holdings
        pot.me?.valueMicros = shareMicros
        return pot
    }

    func testTheFrameFixtureReadsLikeTheFrame() {
        let rows = CabalTreasuryRows(.sampleACCapital)

        XCTAssertEqual(rows.cash, .init(label: "Cash \u{00B7} USDC", apy: "3.5% APY", amount: "$5,100.00"))
        XCTAssertEqual(rows.positions.map(\.name), ["NVIDIA", "Apple", "SpaceX"])
        XCTAssertEqual(rows.positions.map(\.value), ["$8,360.00", "$4,320.00", "$2,220.00"])
        XCTAssertEqual(rows.cashOut, "$7,000")
    }

    func testAverageCostIsTheCostBasisOverTheUnitsInIntegerMicros() {
        let rows = CabalTreasuryRows(.sampleACCapital)

        XCTAssertEqual(rows.positions[0].detail, "38 sh \u{00B7} avg $192.95")
        XCTAssertEqual(rows.positions[1].detail, "24 sh \u{00B7} avg $183.30")
    }

    func testTheReturnIsPnlOverCostInBasisPoints() {
        let rows = CabalTreasuryRows(.sampleACCapital)

        XCTAssertEqual(rows.positions[0].trailing, .change(text: "+14.0%", direction: .up))
        XCTAssertEqual(rows.positions[1].trailing, .change(text: "\u{2212}1.8%", direction: .down))
    }

    func testAPreIpoPositionReadsAtEstimateInsteadOfAReturn() {
        let rows = CabalTreasuryRows(.sampleACCapital)

        XCTAssertEqual(rows.positions[2].detail, "Pre-IPO \u{00B7} 12 sh")
        XCTAssertEqual(rows.positions[2].trailing, .estimate("at estimate"))
    }

    func testFractionalSharesShowUpToTwoDecimals() {
        let rows = CabalTreasuryRows(
            pot([
                holding(symbol: "NVDAx", units: "1.8230", value: 400_000_000, cost: 399_000_000, pnl: 1_000_000),
                holding(symbol: "AAPLx", units: "2.5000", value: 300_000_000, cost: 300_000_000, pnl: 0),
                holding(symbol: "METAx", units: "0.0040", value: 100_000_000, cost: 100_000_000, pnl: 0),
            ]))

        let shares: [String] = rows.positions.map { position in
            let parts: [Substring] = position.detail.split(separator: "\u{00B7}")
            return String(parts.first ?? "").trimmingCharacters(in: .whitespaces)
        }
        XCTAssertEqual(shares, ["1.82 sh", "2.5 sh", "<0.01 sh"])
    }

    func testPositionsSortByValueDescending() {
        let rows = CabalTreasuryRows(
            pot([
                holding(symbol: "AAPLx", units: "1", value: 100_000_000, cost: 90_000_000, pnl: 10_000_000),
                holding(symbol: "NVDAx", units: "1", value: 300_000_000, cost: 90_000_000, pnl: 10_000_000),
                holding(symbol: "METAx", units: "1", value: 200_000_000, cost: 90_000_000, pnl: 10_000_000),
            ]))

        XCTAssertEqual(rows.positions.map(\.symbol), ["NVDAx", "METAx", "AAPLx"])
    }

    func testThereIsNoCashOutRowWithoutAShare() {
        XCTAssertNil(CabalTreasuryRows(pot([], shareMicros: 0)).cashOut)
        var outsider: Pot = .sampleACCapital
        outsider.me = nil
        XCTAssertNil(CabalTreasuryRows(outsider).cashOut)
    }

    func testAPositionWithNoCostBasisHasNoReturnAndNoAverage() {
        let rows = CabalTreasuryRows(
            pot([holding(symbol: "NVDAx", units: "3", value: 600_000_000, cost: 0, pnl: 0)]))

        XCTAssertEqual(rows.positions[0].detail, "3 sh")
        XCTAssertEqual(rows.positions[0].trailing, .none)
    }

    func testTheHistoryLineIsTheMonthMove() {
        let move = CabalHeaderCopy.Move(percent: "\u{2212}4.1%", direction: .down)

        XCTAssertEqual(CabalTreasuryRows.history(move), "\u{2212}4.1% \u{00B7} 1M")
        XCTAssertNil(CabalTreasuryRows.history(nil))
    }

    func testLogosAttachBySymbol() throws {
        let url = try XCTUnwrap(URL(string: "https://cdn.example.com/NVDAx.png"))
        let rows = CabalTreasuryRows(.sampleACCapital).withLogos(["NVDAx": url])

        XCTAssertEqual(rows.positions.map(\.logoURL), [url, nil, nil])
    }
}
