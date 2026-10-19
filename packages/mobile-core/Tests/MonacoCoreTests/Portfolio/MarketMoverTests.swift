import XCTest

@testable import MonacoCore

final class MarketMoverTests: XCTestCase {
    func testTheLargestAbsoluteMoveWins() {
        let assets = [asset("AAA", bps: 120), asset("BBB", bps: -450), asset("CCC", bps: 418)]

        XCTAssertEqual(MarketMover.top(in: assets)?.symbol, "BBB")
    }

    func testAssetsWithNoMoveAreSkipped() {
        XCTAssertEqual(MarketMover.top(in: [asset("AAA", bps: nil), asset("BBB", bps: 5)])?.symbol, "BBB")
        XCTAssertNil(MarketMover.top(in: [asset("AAA", bps: nil)]))
        XCTAssertNil(MarketMover.top(in: []))
    }

    private func asset(_ symbol: String, bps: Int64?) -> MarketAsset {
        MarketAsset(
            symbol: symbol, ticker: symbol, name: symbol, issuer: .xstocks, kind: .stock,
            logoURL: nil, priceMicros: 1_000_000, priceText: "$1.00", changeBasisPoints: bps, changeText: "",
            sparkline: nil, session: .open,
            status: MarketStatus(session: .open, isOpen: true, afterHours: false),
            showsSessionChip: false, isTradable: true)
    }
}
