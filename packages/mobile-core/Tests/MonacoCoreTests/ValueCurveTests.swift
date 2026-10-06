import Foundation
import MonacoAPI
import XCTest

@testable import MonacoCore

final class ValueCurveTests: XCTestCase {
    private typealias MyHistory = Components.Schemas.MyPnlHistory
    private typealias PotHistory = Components.Schemas.CabalValueHistory

    private var mine: ValueCurve { ValueCurve(MyHistory.sample()) }
    private var pot: ValueCurve { ValueCurve(PotHistory.sample()) }

    func testAPnLHistoryDrawsEquityAtEachBucket() {
        let curve = mine

        XCTAssertEqual(curve.points.map(\.value), [100_000_000, 101_500_000, 103_000_000, 104_500_000])
        XCTAssertEqual(curve.points.map(\.at), curve.samples.map(\.at))
        XCTAssertTrue(curve.hasEnoughHistory)
    }

    func testACabalHistoryDrawsThePotValue() {
        let curve = pot

        XCTAssertEqual(curve.points.map(\.value), [500_000_000, 505_000_000, 498_000_000, 510_000_000])
    }

    func testTheReadoutDefaultsToTheNewestPoint() {
        let readout = mine.readout()

        XCTAssertEqual(readout?.value, "$104.50")
        XCTAssertEqual(readout?.pnl, "+$4.50")
        XCTAssertEqual(readout?.direction, .up)
    }

    func testScrubbingReadsTheValueAndPnLUnderTheFinger() {
        let curve = mine

        XCTAssertEqual(curve.readout(at: 1)?.value, "$101.50")
        XCTAssertEqual(curve.readout(at: 1)?.pnl, "+$1.50")
        XCTAssertEqual(curve.readout(at: 0)?.pnl, "$0.00")
        XCTAssertEqual(curve.readout(at: 0)?.direction, .flat)
        XCTAssertEqual(curve.readout(at: 1)?.at, curve.samples[1].at)
    }

    func testALossReadsWithATypographicMinus() {
        let curve = pot

        XCTAssertEqual(curve.readout(at: 2)?.value, "$498.00")
        XCTAssertEqual(curve.readout(at: 2)?.pnl, "−$2.00")
        XCTAssertEqual(curve.readout(at: 2)?.direction, .down)
    }

    func testAnIndexOutsideTheCurveHasNoReadout() {
        let curve = mine

        XCTAssertNil(curve.readout(at: 4))
        XCTAssertNil(curve.readout(at: -1))
        XCTAssertNil(ValueCurve(MyHistory.sampleEmpty).readout())
    }

    func testTheNearestIndexIsTheSampleClosestInTime() {
        let curve = mine
        let between = curve.samples[1].at.addingTimeInterval(2_000)

        XCTAssertEqual(curve.nearestIndex(to: between), 2)
        XCTAssertEqual(curve.nearestIndex(to: .distantPast), 0)
        XCTAssertEqual(curve.nearestIndex(to: .distantFuture), 3)
        XCTAssertNil(ValueCurve(MyHistory.sampleEmpty).nearestIndex(to: between))
    }

    func testFewerThanTwoPointsIsNotEnoughHistoryAndFlat() {
        let short = ValueCurve(PotHistory.sampleShort())

        XCTAssertFalse(short.hasEnoughHistory)
        XCTAssertEqual(short.direction, .flat)
        XCTAssertFalse(ValueCurve(MyHistory.sampleEmpty).hasEnoughHistory)
    }

    func testTheWindowDirectionFollowsTheServersPnLAtEachEnd() {
        XCTAssertEqual(mine.direction, .up)
        let down = ValueCurve(samples: [
            .init(at: Date(timeIntervalSince1970: 0), value: 10, pnl: 5),
            .init(at: Date(timeIntervalSince1970: 1), value: 9, pnl: 4),
        ])
        XCTAssertEqual(down.direction, .down)
    }
}
