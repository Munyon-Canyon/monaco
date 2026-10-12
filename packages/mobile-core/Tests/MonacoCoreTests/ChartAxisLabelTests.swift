import XCTest

@testable import MonacoCore

final class ChartAxisLabelTests: XCTestCase {
    private let utc = TimeZone(identifier: "UTC") ?? .gmt
    private let us = Locale(identifier: "en_US")
    private let gb = Locale(identifier: "en_GB")
    private let tuesday = Date(timeIntervalSince1970: 1_790_085_600 + 5 * 300)

    private func label(_ seconds: Int, boundary: Bool = false, locale: Locale? = nil) -> String {
        ChartAxisLabel.text(
            tuesday,
            intervalSeconds: seconds,
            dayBoundary: boundary,
            locale: locale ?? us,
            timeZone: utc
        )
    }

    func testTemplate_perInterval() {
        XCTAssertEqual(ChartAxisLabel.template(intervalSeconds: 300, dayBoundary: false), "jmm")
        XCTAssertEqual(ChartAxisLabel.template(intervalSeconds: 900, dayBoundary: false), "jmm")
        XCTAssertEqual(ChartAxisLabel.template(intervalSeconds: 1_800, dayBoundary: false), "jmm")
        XCTAssertEqual(ChartAxisLabel.template(intervalSeconds: 3_600, dayBoundary: false), "EEEjmm")
        XCTAssertEqual(ChartAxisLabel.template(intervalSeconds: 14_400, dayBoundary: false), "EEEjmm")
        XCTAssertEqual(ChartAxisLabel.template(intervalSeconds: 86_400, dayBoundary: false), "MMMd")
        XCTAssertEqual(ChartAxisLabel.template(intervalSeconds: 604_800, dayBoundary: false), "MMMd")
    }

    func testTemplate_dayBoundaryOverridesOnlyUpToAnHour() {
        XCTAssertEqual(ChartAxisLabel.template(intervalSeconds: 300, dayBoundary: true), "MMMd")
        XCTAssertEqual(ChartAxisLabel.template(intervalSeconds: 1_800, dayBoundary: true), "MMMd")
        XCTAssertEqual(ChartAxisLabel.template(intervalSeconds: 3_600, dayBoundary: true), "MMMd")
        XCTAssertEqual(ChartAxisLabel.template(intervalSeconds: 14_400, dayBoundary: true), "EEEjmm")
        XCTAssertEqual(ChartAxisLabel.template(intervalSeconds: 86_400, dayBoundary: true), "MMMd")
    }

    func testText_intradayReadsTheClockInTheLocalesOwnStyle() {
        XCTAssertEqual(label(900).replacingOccurrences(of: "\u{202F}", with: " "), "2:25 PM")
        XCTAssertEqual(label(900, locale: gb), "14:25")
    }

    func testText_hourlyIncludesTheWeekday() {
        XCTAssertTrue(label(3_600).contains("Tue"))
        XCTAssertTrue(label(14_400).contains("2:25"))
    }

    func testText_dailyAndBoundaryReadTheDate() {
        XCTAssertEqual(label(86_400), "Sep 22")
        XCTAssertEqual(label(900, boundary: true), "Sep 22")
    }

    func testIsDayStart_isTrueOnlyForTheFirstStepOfTheLocalDay() {
        let midnight = Date(timeIntervalSince1970: 1_790_035_200)
        XCTAssertTrue(ChartAxisLabel.isDayStart(midnight, withinSeconds: 3_600, timeZone: utc))
        XCTAssertTrue(
            ChartAxisLabel.isDayStart(midnight.addingTimeInterval(1_800), withinSeconds: 3_600, timeZone: utc))
        XCTAssertFalse(
            ChartAxisLabel.isDayStart(midnight.addingTimeInterval(3_600), withinSeconds: 3_600, timeZone: utc))
        XCTAssertFalse(ChartAxisLabel.isDayStart(tuesday, withinSeconds: 3_600, timeZone: utc))
    }
}

final class ChartAxisTicksTests: XCTestCase {
    private let utc = TimeZone(identifier: "UTC") ?? .gmt
    private let us = Locale(identifier: "en_US")
    private let midnight = Date(timeIntervalSince1970: 1_790_035_200)

    func testStep_picksTheSmallestNiceStepThatKeepsAboutSixLabels() {
        XCTAssertEqual(ChartAxisTicks.step(visibleSeconds: 5 * 3_600), 3_600)
        XCTAssertEqual(ChartAxisTicks.step(visibleSeconds: 12 * 3_600), 10_800)
        XCTAssertEqual(ChartAxisTicks.step(visibleSeconds: 60 * 3_600), 43_200)
        XCTAssertEqual(ChartAxisTicks.step(visibleSeconds: 30 * 86_400), 604_800)
        XCTAssertEqual(ChartAxisTicks.step(visibleSeconds: 10_000 * 86_400), 31_536_000)
    }

    func testDates_alignToTheLocalClockAndStayInsideTheWindow() {
        let start = midnight.addingTimeInterval(3_600 + 5)
        let end = midnight.addingTimeInterval(13 * 3_600)

        let ticks = ChartAxisTicks.dates(from: start, to: end, step: 10_800, timeZone: utc)

        XCTAssertEqual(ticks.map { $0.timeIntervalSince(midnight) / 3_600 }, [3, 6, 9, 12])
    }

    func testDates_followAnOffsetTimeZone() {
        let plusTwo = TimeZone(secondsFromGMT: 7_200) ?? .gmt
        let ticks = ChartAxisTicks.dates(
            from: midnight, to: midnight.addingTimeInterval(86_400), step: 86_400, timeZone: plusTwo)

        XCTAssertEqual(ticks.map { $0.timeIntervalSince(midnight) }, [79_200])
    }

    func testDates_areBoundedAndEmptyForABackwardsWindow() {
        XCTAssertTrue(ChartAxisTicks.dates(from: midnight, to: midnight, step: 300, timeZone: utc).isEmpty)
        XCTAssertLessThanOrEqual(
            ChartAxisTicks.dates(from: midnight, to: midnight.addingTimeInterval(86_400 * 30), step: 300, timeZone: utc)
                .count, 24)
    }

    func testText_readsTheClockInsideADayAndTheDateAtMidnightAndAcrossDays() {
        XCTAssertEqual(
            ChartAxisTicks.text(midnight.addingTimeInterval(3 * 3_600), step: 10_800, locale: us, timeZone: utc)
                .replacingOccurrences(of: "\u{202F}", with: " "), "3:00 AM")
        XCTAssertEqual(ChartAxisTicks.text(midnight, step: 10_800, locale: us, timeZone: utc), "Sep 22")
        XCTAssertEqual(
            ChartAxisTicks.text(midnight.addingTimeInterval(9 * 3_600), step: 86_400, locale: us, timeZone: utc),
            "Sep 22")
        XCTAssertEqual(ChartAxisTicks.text(midnight, step: 15_552_000, locale: us, timeZone: utc), "Sep 2026")
    }

    func testFitting_dropsALabelThatWouldCrossTheEitherEdgeOfThePlot() {
        let scroll = midnight
        let visible: TimeInterval = 1_000
        let dates = [0, 30, 500, 960, 1_000].map { scroll.addingTimeInterval(TimeInterval($0)) }

        let kept = ChartAxisTicks.fitting(
            dates, scrollDate: scroll, visibleSeconds: visible, plotWidth: 300, labelWidth: { _ in 60 })

        XCTAssertEqual(kept.map { $0.timeIntervalSince(scroll) }, [500])
    }

    func testFitting_keepsALabelThatJustFitsAndHandlesAnEmptyPlot() {
        let scroll = midnight
        let edge = scroll.addingTimeInterval(100)
        let kept = ChartAxisTicks.fitting(
            [edge], scrollDate: scroll, visibleSeconds: 1_000, plotWidth: 300, labelWidth: { _ in 60 })
        XCTAssertEqual(kept, [edge])
        XCTAssertTrue(
            ChartAxisTicks.fitting(
                [edge], scrollDate: scroll, visibleSeconds: 1_000, plotWidth: 0, labelWidth: { _ in 1 }
            )
            .isEmpty)
    }

    func testText_weeklyBarsReadDatesAtEveryZoom() {
        let monday = Date(timeIntervalSince1970: 1_790_035_200 - 86_400)
        let sixMonths = ChartAxisTicks.step(visibleSeconds: 26 * 604_800)
        let year = ChartAxisTicks.step(visibleSeconds: 52 * 604_800)

        XCTAssertEqual(ChartAxisTicks.text(monday, step: sixMonths, locale: us, timeZone: utc), "Sep 21")
        XCTAssertEqual(ChartAxisTicks.text(monday, step: year, locale: us, timeZone: utc), "Sep 21")
        XCTAssertEqual(ChartAxisTicks.text(monday, step: 604_800, locale: us, timeZone: utc), "Sep 21")
    }

    func testDates_forWeeklyBarsLandOnMondays() {
        let monday = Date(timeIntervalSince1970: 1_790_035_200 - 86_400)
        let step = 2 * 604_800.0
        let anchor = ChartAxisTicks.anchor(forBarSeconds: 604_800, step: step)

        let ticks = ChartAxisTicks.dates(
            from: monday.addingTimeInterval(-40 * 86_400), to: monday.addingTimeInterval(40 * 86_400), step: step,
            anchor: anchor, timeZone: utc)

        let weekdays = Set(
            ticks.map { Calendar(identifier: .gregorian).dateComponents(in: utc, from: $0).weekday ?? 0 })
        XCTAssertEqual(weekdays, [2])
        XCTAssertEqual(ChartAxisTicks.anchor(forBarSeconds: 86_400, step: step), 0)
        XCTAssertEqual(ChartAxisTicks.anchor(forBarSeconds: 604_800, step: 2_592_000), 0)
    }
}
