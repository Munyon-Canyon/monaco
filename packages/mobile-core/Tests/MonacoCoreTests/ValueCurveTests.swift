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

    func testOnePointDrawsAFlatLineEndingAtThatSample() {
        let short = ValueCurve(PotHistory.sampleShort())
        let only = short.samples[0]

        XCTAssertTrue(short.hasEnoughHistory)
        XCTAssertEqual(short.direction, .flat)
        XCTAssertEqual(short.drawnPoints.map(\.value), [only.value, only.value])
        XCTAssertEqual(short.drawnPoints.map(\.at), [only.at.addingTimeInterval(-1), only.at])
        XCTAssertEqual(short.drawnNavPoints.map(\.value), [1_000_000, 1_000_000])
        XCTAssertEqual(short.readout()?.value, "$50.00")
        XCTAssertEqual(short.nearestIndex(to: only.at.addingTimeInterval(-1)), 0)
    }

    func testAOnePointCurveAnswersForEveryDrawnPoint() {
        let only = ValueCurve.Sample(at: Date(timeIntervalSince1970: 1_790_931_600), value: 50_000_000, pnl: 1_500_000)
        let short = ValueCurve(samples: [only])

        XCTAssertEqual(short.drawnPoints.count, 2)
        for index in short.drawnPoints.indices {
            XCTAssertEqual(short.readout(at: index)?.value, "$50.00", "readout at \(index)")
            XCTAssertEqual(short.readout(at: index)?.pnl, "+$1.50", "readout at \(index)")
        }
        XCTAssertEqual(short.readout(at: 1)?.at, only.at)
        XCTAssertNil(short.readout(at: 2))
        XCTAssertEqual(short.nearestIndex(to: only.at), 1)
    }

    func testTwoOrMorePointsAreDrawnAsTheyAre() {
        XCTAssertEqual(mine.drawnPoints, mine.points)
        XCTAssertEqual(pot.drawnNavPoints, pot.navPoints)
    }

    func testNoPointsIsNotEnoughHistory() {
        let empty = ValueCurve(MyHistory.sampleEmpty)

        XCTAssertFalse(empty.hasEnoughHistory)
        XCTAssertTrue(empty.drawnPoints.isEmpty)
    }

    func testTheWindowDirectionFollowsTheServersPnLAtEachEnd() {
        XCTAssertEqual(mine.direction, .up)
        let down = ValueCurve(samples: [
            .init(at: Date(timeIntervalSince1970: 0), value: 10, pnl: 5),
            .init(at: Date(timeIntervalSince1970: 1), value: 9, pnl: 4),
        ])
        XCTAssertEqual(down.direction, .down)
    }

    func testACabalCurveCarriesTheNavPerShareForTheReturnLines() {
        XCTAssertEqual(pot.navPoints.map(\.value), [1_000_000, 1_000_000, 1_000_000, 1_000_000])
        XCTAssertTrue(mine.navPoints.isEmpty)
    }

    func testTheShortHistoryLineNamesTheWindow() {
        XCTAssertEqual(LeaderboardRange.oneMonth.shortHistoryLine, "Not enough history for the last month yet")
        XCTAssertEqual(LeaderboardRange.oneDay.shortHistoryLine, "Not enough history for the last day yet")
        XCTAssertEqual(LeaderboardRange.all.shortHistoryLine, "Not enough history yet")
    }
}
