import Foundation
import XCTest

@testable import MonacoCore

final class ValueCurvePlotTests: XCTestCase {
    private let t0 = Date(timeIntervalSince1970: 1_800_000_000)

    private func curve(_ rows: [(value: Int64, pnl: Int64, nav: Int64?)]) -> ValueCurve {
        ValueCurve(
            samples: rows.enumerated().map {
                ValueCurve.Sample(
                    at: t0.addingTimeInterval(Double($0.offset) * 600), value: $0.element.value, pnl: $0.element.pnl,
                    nav: $0.element.nav)
            })
    }

    private func points(_ values: [Int64]) -> [CurvePoint] {
        values.enumerated().map { CurvePoint(at: t0.addingTimeInterval(Double($0.offset)), value: $0.element) }
    }

    func testPlottedPnLAndNavWithFallback() {
        let value = curve([(3_000_000, 10_000, 1_000_000), (0, 12_000, nil)])

        XCTAssertEqual(value.plotted(.pnl).map(\.value), [10_000, 12_000])
        XCTAssertEqual(value.plotted(.nav).map(\.value), [1_000_000, 12_000])
    }

    func testPlottedDrawsASinglePointAsALine() {
        XCTAssertEqual(curve([(1, 5, nil)]).plotted(.pnl).map(\.value), [5, 5])
    }

    func testFittedDomainPadsBySpanAndSkipsZero() {
        let domain = ValueCurve.fittedDomain(points([3_000_000, 3_010_000]))

        XCTAssertEqual(domain.lowerBound, 3_000_000 - 800)
        XCTAssertEqual(domain.upperBound, 3_010_000 + 800)
        XCTAssertFalse(domain.contains(0))
    }

    func testFittedDomainForFlatSingleAndEmptySeries() {
        XCTAssertEqual(ValueCurve.fittedDomain(points([5, 5, 5])), (5 - 1_000)...(5 + 1_000))
        XCTAssertEqual(ValueCurve.fittedDomain(points([4_000_000_000])), 3_980_000_000...4_020_000_000)
        XCTAssertEqual(ValueCurve.fittedDomain([]), 0...1)
    }

    func testPnLChangeHasNoCliffWhenEquityDropsToZero() {
        let value = curve([(3_000_000, 0, nil), (0, 10_000, nil), (0, 10_000, nil), (3_000_000, 20_000, nil)])

        XCTAssertEqual(value.plotted(.pnl).map(\.value), [0, 10_000, 10_000, 20_000])
        XCTAssertEqual(
            value.change(.pnl),
            ValueCurve.Change(amountText: "+$0.02", percentText: "+0.33%", direction: .up))
        XCTAssertEqual(value.change(.pnl, to: 1)?.amountText, "+$0.01")
        XCTAssertEqual(value.change(.pnl, to: 1)?.percentText, "+0.33%")
    }

    func testPnLChangeUnderACentReadsThePercentAlone() {
        let value = curve([(3_000_000, 0, nil), (3_000_000, 3_000, nil)])

        XCTAssertEqual(
            value.change(.pnl), ValueCurve.Change(amountText: nil, percentText: "+0.10%", direction: .up))
    }

    func testPnLChangeThatRoundsToNothingIsFlat() {
        let value = curve([(3_000_000, 0, nil), (3_000_000, 100, nil)])

        XCTAssertEqual(
            value.change(.pnl), ValueCurve.Change(amountText: "$0.00", percentText: "0.00%", direction: .flat))
    }

    func testHomePillReadsTheAllTimePercentAloneWhenDollarsRoundToZero() {
        let value = curve([(3_003_000, 0, nil), (3_000_000, -3_000, nil)])

        XCTAssertEqual(value.pill(.pnl), ValueCurve.Pill(text: "−0.1%", direction: .down))
    }

    func testHomePillReadsSignedDollarsOtherwiseAndFlatWhenNothingMoved() {
        XCTAssertEqual(
            curve([(3_000_000, 0, nil), (3_050_000, 50_000, nil)]).pill(.pnl),
            ValueCurve.Pill(text: "+$0.05", direction: .up))
        XCTAssertEqual(
            curve([(3_000_000, 0, nil), (3_000_000, 100, nil)]).pill(.pnl),
            ValueCurve.Pill(text: "$0.00", direction: .flat))
    }

    func testCabalPillIsTheReturnPercentNeverTheFourDecimalSharePrice() {
        let value = curve([(1_000_000, 0, 1_000_000), (1_000_000, 0, 1_003_100)])

        XCTAssertEqual(value.pill(.nav), ValueCurve.Pill(text: "+0.3%", direction: .up))
        XCTAssertNil(curve([(1_000_000, 0, nil)]).pill(.nav))
    }

    func testPnLLossReadsWithATypographicMinus() {
        let value = curve([(4_000_000, 0, nil), (4_000_000, -20_000, nil)])

        XCTAssertEqual(
            value.change(.pnl), ValueCurve.Change(amountText: "−$0.02", percentText: "−0.50%", direction: .down))
    }

    func testPnLPercentIsNilWithoutPositiveEquity() {
        let value = curve([(0, 0, nil), (0, 10_000, nil)])

        XCTAssertEqual(value.change(.pnl), ValueCurve.Change(amountText: "+$0.01", percentText: nil, direction: .up))
    }

    func testNavChangeIsAPercentOnly() {
        let value = curve([(1, 0, 1_000_000), (1, 0, 1_003_100), (1, 0, 999_500)])

        XCTAssertEqual(
            value.change(.nav), ValueCurve.Change(amountText: nil, percentText: "−0.05%", direction: .down))
        XCTAssertEqual(value.change(.nav, to: 1)?.percentText, "+0.31%")
        XCTAssertEqual(value.change(.nav, to: 0)?.direction, .flat)
    }

    func testNavPercentIsNilWithoutABase() {
        XCTAssertNil(curve([(1, 0, nil), (1, 5, nil)]).change(.nav)?.percentText)
    }

    func testChangeIsNilForMissingIndexOrEmptyCurve() {
        XCTAssertNil(curve([]).change(.pnl))
        XCTAssertNil(curve([(1, 1, nil)]).change(.pnl, to: 5))
    }
}
