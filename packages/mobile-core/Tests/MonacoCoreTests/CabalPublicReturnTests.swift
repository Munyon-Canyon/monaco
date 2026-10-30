import Foundation
import MonacoCore
import XCTest

final class CabalPublicReturnTests: XCTestCase {
    private func curve(_ samples: [ValueCurve.Sample]) -> ValueCurve {
        ValueCurve(samples: samples)
    }

    private func sample(_ day: Double, nav: Int64?, pnl: Int64) -> ValueCurve.Sample {
        ValueCurve.Sample(at: Date(timeIntervalSince1970: day * 86_400), value: 1_000_000, pnl: pnl, nav: nav)
    }

    func testAnUnloadedCurveIsPending() {
        XCTAssertEqual(CabalPublicReturn(nil), .pending)
    }

    func testACurveWithNoPointsSaysNoReturnYet() {
        XCTAssertEqual(CabalPublicReturn(curve([])), .none)
    }

    func testTheReturnIsLastNavOverFirstNavAndThePnlIsTheChange() {
        let shown = CabalPublicReturn(
            curve([
                sample(0, nav: 1_000_000, pnl: 5_000_000),
                sample(10, nav: 1_050_000, pnl: 20_000_000),
                sample(20, nav: 1_123_400, pnl: 62_500_000),
            ]))
        XCTAssertEqual(shown, .value(percent: "+12.3%", pnl: "+$57.50"))
    }

    func testALossReadsWithTheTypographicMinus() {
        let shown = CabalPublicReturn(
            curve([
                sample(0, nav: 1_000_000, pnl: 0),
                sample(10, nav: 960_000, pnl: -40_000_000),
            ]))
        XCTAssertEqual(shown, .value(percent: "\u{2212}4.0%", pnl: "\u{2212}$40.00"))
    }

    func testASinglePointReadsFlat() {
        let shown = CabalPublicReturn(curve([sample(0, nav: 1_000_000, pnl: 0)]))
        XCTAssertEqual(shown, .value(percent: "0.0%", pnl: "$0.00"))
    }

    func testPointsWithoutNavSayNoReturnYet() {
        XCTAssertEqual(CabalPublicReturn(curve([sample(0, nav: nil, pnl: 0)])), .none)
    }
}
