import XCTest

@testable import MonacoCore

final class ValueCurveChartTests: XCTestCase {
    private let start = Date(timeIntervalSince1970: 1_790_085_600)

    private func curve(_ values: [Int64], step: TimeInterval = 300) -> ValueCurve {
        ValueCurve(
            samples: values.enumerated().map { index, value in
                .init(at: start.addingTimeInterval(TimeInterval(index) * step), value: value, pnl: value - values[0])
            })
    }

    func testChartBars_areFlatBarsInDollarsAtTheSamplesTimes() {
        let bars = curve([1_000_000_000, 1_014_500_000]).chartBars

        XCTAssertEqual(bars.map(\.close), [1_000, 1_014.5])
        XCTAssertTrue(bars.allSatisfy { $0.open == $0.close && $0.high == $0.close && $0.low == $0.close })
        XCTAssertEqual(bars.map(\.date), [start, start.addingTimeInterval(300)])
    }

    func testChartBars_indexTheSameWayTheReadoutAndTheNearestSearchDo() throws {
        let value = curve([100_000_000, 130_000_000, 120_000_000])
        let held = try XCTUnwrap(value.nearestIndex(to: start.addingTimeInterval(290)))

        XCTAssertEqual(held, 1)
        XCTAssertEqual(value.chartBars[held].close, 130)
        XCTAssertEqual(value.readout(at: held)?.value, "$130.00")
    }

    func testChartBars_aSingleSampleStillDrawsTwoBars() {
        let value = curve([250_000_000])

        XCTAssertEqual(value.chartBars.count, 2)
        XCTAssertEqual(value.chartBars.map(\.close), [250, 250])
        XCTAssertGreaterThanOrEqual(value.slotSeconds, 1)
    }

    func testFittedDomainContainsTheFirstPointSoTheBaselineAlwaysDraws() throws {
        for values: [Int64] in [[0, 1_000_000, 3_000_000], [0, -2_000_000, -5_000_000], [4_000_000]] {
            let plotted = curve(values).plotted(.pnl)
            let first = try XCTUnwrap(plotted.first).value
            XCTAssertTrue(ValueCurve.fittedDomain(plotted).contains(first), "\(values)")
        }
    }

    func testSlotSeconds_isTheMedianGapSoAnOddSnapshotDoesNotSetTheBarWidth() {
        let value = ValueCurve(
            samples: [0, 300, 600, 900, 4_500].map { .init(at: start.addingTimeInterval($0), value: 1, pnl: 0) })

        XCTAssertEqual(value.slotSeconds, 300)
        XCTAssertEqual(ValueCurve(samples: []).slotSeconds, 60)
    }
}

final class ChartCrosshairLabelTests: XCTestCase {
    private let utc = TimeZone(identifier: "UTC") ?? .gmt
    private let us = Locale(identifier: "en_US")
    private let noon = Date(timeIntervalSince1970: 1_790_085_600 - 2 * 3_600)

    private func text(visible: TimeInterval, interval: Int) -> String {
        ChartCrosshairLabel.text(noon, visibleSeconds: visible, intervalSeconds: interval, locale: us, timeZone: utc)
            .replacingOccurrences(of: "\u{202F}", with: " ")
    }

    func testAnIntradayWindowReadsTheClock() {
        XCTAssertEqual(text(visible: 86_400, interval: 300), "12:00 PM")
    }

    func testAMultiDayWindowOfDailyBarsReadsTheDate() {
        XCTAssertEqual(text(visible: 30 * 86_400, interval: 86_400), "Sep 22")
    }

    func testAMultiDayWindowOfIntradayBarsReadsDateAndClock() {
        XCTAssertTrue(text(visible: 7 * 86_400, interval: 3_600).hasPrefix("Sep 22"))
        XCTAssertTrue(text(visible: 7 * 86_400, interval: 3_600).contains("12:00"))
    }
}
