import XCTest

@testable import MonacoCore

final class AssetChartBarsTests: XCTestCase {
    private let start: Int64 = 1_790_085_600

    private func point(_ index: Int, step: Int64 = 900, candle: Bool = true) -> MarketChartPoint {
        let close = 100_000_000 + Int64(index) * 1_000_000
        return MarketChartPoint(
            timestamp: start + Int64(index) * step,
            priceUsdcMicros: close,
            openUsdcMicros: candle ? close - 500_000 : 0,
            highUsdcMicros: candle ? close + 1_000_000 : 0,
            lowUsdcMicros: candle ? close - 1_500_000 : 0
        )
    }

    private func series(
        _ indices: Range<Int>,
        range: AssetChartRange = .oneWeek,
        interval: ChartInterval = .m15,
        nextBefore: Date? = nil,
        candle: Bool = true
    ) -> AssetChartSeries {
        AssetChartSeries(
            range: range,
            points: indices.map { point($0, candle: candle) },
            previousCloseUsdcMicros: 99_000_000,
            interval: interval,
            nextBefore: nextBefore
        )
    }

    func testBars_carryTheOpenHighLowCloseInDollars() throws {
        let bar = try XCTUnwrap(series(0..<3).bar(at: 1))

        XCTAssertEqual(bar.open, 100.5, accuracy: 1e-9)
        XCTAssertEqual(bar.high, 102, accuracy: 1e-9)
        XCTAssertEqual(bar.low, 99.5, accuracy: 1e-9)
        XCTAssertEqual(bar.close, 101, accuracy: 1e-9)
        XCTAssertTrue(bar.isRising)
        XCTAssertEqual(bar.date, Date(timeIntervalSince1970: TimeInterval(start + 900)))
    }

    func testBars_fallBackToTheCloseWhenThePointHasNoCandle() throws {
        let bar = try XCTUnwrap(series(0..<2, candle: false).bar(at: 0))

        XCTAssertEqual(bar.open, 100, accuracy: 1e-9)
        XCTAssertEqual(bar.high, 100, accuracy: 1e-9)
        XCTAssertEqual(bar.low, 100, accuracy: 1e-9)
    }

    func testBars_wickAlwaysContainsTheBody() throws {
        let odd = MarketChartPoint(
            timestamp: start,
            priceUsdcMicros: 105_000_000,
            openUsdcMicros: 101_000_000,
            highUsdcMicros: 104_000_000,
            lowUsdcMicros: 102_000_000
        )
        let bar = try XCTUnwrap(AssetChartSeries(range: .oneDay, points: [odd]).bar(at: 0))

        XCTAssertEqual(bar.high, 105, accuracy: 1e-9)
        XCTAssertEqual(bar.low, 102, accuracy: 1e-9)
    }

    func testBars_areNotThinnedAndStayAscending() {
        let long = series(0..<1_500)

        XCTAssertEqual(long.bars.count, 1_500)
        XCTAssertEqual(long.bars, long.bars.sorted { $0.date < $1.date })
        XCTAssertNil(long.bar(at: 1_500))
        XCTAssertNil(long.bar(at: -1))
    }

    func testInterval_defaultsFromTheRangeAndKeepsAnExplicitOne() {
        XCTAssertEqual(AssetChartSeries(range: .oneDay, points: []).interval, .m5)
        XCTAssertEqual(AssetChartSeries(range: .oneYear, points: []).interval, .d1)
        XCTAssertEqual(series(0..<2).interval, .m15)
        XCTAssertEqual(series(0..<2).intervalSeconds, 900)
    }

    func testNearestBarIndex_takesTheCloserBarAndClampsOutsideTheSeries() {
        let chart = series(0..<10)
        let first = Date(timeIntervalSince1970: TimeInterval(start))

        XCTAssertEqual(chart.nearestBarIndex(to: first.addingTimeInterval(-5_000)), 0)
        XCTAssertEqual(chart.nearestBarIndex(to: first.addingTimeInterval(899 * 3)), 3)
        XCTAssertEqual(chart.nearestBarIndex(to: first.addingTimeInterval(900 * 3 + 400)), 3)
        XCTAssertEqual(chart.nearestBarIndex(to: first.addingTimeInterval(900 * 3 + 460)), 4)
        XCTAssertEqual(chart.nearestBarIndex(to: first.addingTimeInterval(900 * 99)), 9)
        XCTAssertNil(AssetChartSeries(range: .oneDay, points: []).nearestBarIndex(to: first))
    }

    func testChange_toABarMeasuresFromTheBaselineAndNilMeansTheEnd() {
        let chart = series(0..<5, range: .oneDay)

        XCTAssertEqual(chart.changeRatio(toBarIndex: nil), chart.changeRatio())
        XCTAssertEqual(chart.changeRatio(toBarIndex: 4), chart.changeRatio())
        XCTAssertEqual(chart.changeDollars(toBarIndex: 0), "1.00")
        XCTAssertEqual(chart.changeDollars(toBarIndex: 2), "3.00")
        XCTAssertNil(chart.changeRatio(toBarIndex: 5))
        XCTAssertNil(chart.changeDollars(toBarIndex: -1))
    }

    func testPrepending_mergesAscendingDropsTheSharedBarAndTakesTheOlderCursor() {
        let cursor = Date(timeIntervalSince1970: TimeInterval(start - 3_600))
        let newer = series(10..<20, nextBefore: Date(timeIntervalSince1970: TimeInterval(start)))
        let older = series(0..<11, nextBefore: cursor)

        let merged = newer.prepending(older)

        XCTAssertEqual(merged.bars.count, 20)
        XCTAssertEqual(merged.bars.map(\.date), merged.bars.map(\.date).sorted())
        XCTAssertEqual(Set(merged.bars.map(\.date)).count, 20)
        XCTAssertEqual(merged.nextBefore, cursor)
        XCTAssertEqual(merged.range, .oneWeek)
        XCTAssertEqual(merged.interval, .m15)
        XCTAssertEqual(merged.previousCloseUsdcMicros, 99_000_000)
    }

    func testPrepending_aFinalPageClearsTheCursor() {
        let newer = series(10..<20, nextBefore: Date(timeIntervalSince1970: TimeInterval(start)))
        let oldest = series(0..<10, nextBefore: nil)

        XCTAssertNil(newer.prepending(oldest).nextBefore)
        XCTAssertFalse(newer.prepending(oldest).hasOlder)
        XCTAssertTrue(newer.hasOlder)
    }

    func testDescribeBar_readsAllFourPricesAndTheTime() throws {
        let chart = series(0..<3)
        let utc = TimeZone(identifier: "UTC") ?? .gmt

        let text = try XCTUnwrap(chart.describeBar(at: 1, locale: Locale(identifier: "en_GB"), timeZone: utc))

        XCTAssertTrue(text.hasPrefix("Open 100.50, high 102.00, low 99.50, close 101.00, "), text)
        XCTAssertTrue(text.contains("14:15"), text)
        XCTAssertNil(chart.describeBar(at: 3))
    }

    func testCaption_aSubDayIntervalShowsTheClockOnEveryRange() {
        let utc = TimeZone(identifier: "UTC") ?? .gmt
        let date = Date(timeIntervalSince1970: TimeInterval(start))
        let locale = Locale(identifier: "en_GB")

        let week = ChartScrubLabel.caption(
            for: date, range: .oneWeek, intervalSeconds: 900, locale: locale, timeZone: utc)
        let weekDaily = ChartScrubLabel.caption(for: date, range: .oneWeek, locale: locale, timeZone: utc)

        XCTAssertTrue(week.contains("14:00"), week)
        XCTAssertFalse(weekDaily.contains("14:00"), weekDaily)
    }

    func testPrepending_keepsTheRangeBaselineSoThePercentDoesNotMoveWhilePanning() {
        let newer = series(10..<20, nextBefore: Date(timeIntervalSince1970: TimeInterval(start)))
        let older = series(0..<10)

        let merged = newer.prepending(older)

        XCTAssertEqual(newer.baselineUsdcMicros, 110_000_000)
        XCTAssertEqual(merged.baselineUsdcMicros, 110_000_000)
        XCTAssertEqual(merged.changeRatio(), newer.changeRatio())
    }

    func testRefreshed_keepsOlderBarsAndTheirCursorAndTakesTheFreshTail() {
        let cursor = Date(timeIntervalSince1970: TimeInterval(start - 3_600))
        let loaded = series(0..<20, nextBefore: cursor)
        let fresh = series(10..<22, nextBefore: Date(timeIntervalSince1970: TimeInterval(start)))

        let merged = loaded.refreshed(with: fresh)

        XCTAssertEqual(merged.bars.count, 22)
        XCTAssertEqual(merged.nextBefore, cursor)
        XCTAssertEqual(merged.bars.last, fresh.bars.last)
        XCTAssertEqual(merged.baselineUsdcMicros, fresh.baselineUsdcMicros)
    }

    func testRefreshed_aFreshPageThatCoversEverythingReplacesTheSeries() {
        let loaded = series(10..<20)
        let fresh = series(0..<22, nextBefore: Date(timeIntervalSince1970: TimeInterval(start - 900)))

        XCTAssertEqual(loaded.refreshed(with: fresh), fresh)
    }

    private func flat(_ index: Int, _ price: Double) -> PlottedBar {
        PlottedBar(
            date: Date(timeIntervalSince1970: TimeInterval(start + Int64(index) * 3_600)),
            open: price, high: price, low: price, close: price)
    }

    func testCandleBars_aFlatBarAfterAnUpBarOpensAtThePreviousClose() {
        let up = PlottedBar(
            date: Date(timeIntervalSince1970: TimeInterval(start)), open: 100, high: 103, low: 99, close: 102)
        let drawn = [up, flat(1, 102), flat(2, 105)].openingFlatBarsAtThePreviousClose

        XCTAssertEqual(drawn[0], up)
        XCTAssertEqual(drawn[1], flat(1, 102))
        XCTAssertEqual(drawn[2].open, 102)
        XCTAssertEqual(drawn[2].close, 105)
        XCTAssertEqual(drawn[2].high, 105)
        XCTAssertEqual(drawn[2].low, 102)
        XCTAssertTrue(drawn[2].isRising)
    }

    func testCandleBars_aFlatBarAfterADownMoveDrawsAFallingBody() {
        let drawn = [flat(0, 110), flat(1, 104)].openingFlatBarsAtThePreviousClose

        XCTAssertEqual(drawn[0], flat(0, 110))
        XCTAssertEqual(drawn[1].open, 110)
        XCTAssertEqual(drawn[1].high, 110)
        XCTAssertEqual(drawn[1].low, 104)
        XCTAssertFalse(drawn[1].isRising)
    }

    func testCandleBars_theFirstBarStaysFlatAndRealCandlesAreUntouched() {
        let real = PlottedBar(
            date: Date(timeIntervalSince1970: TimeInterval(start + 7_200)), open: 101, high: 104, low: 100, close: 103)
        let drawn = [flat(0, 100), flat(1, 100), real].openingFlatBarsAtThePreviousClose

        XCTAssertEqual(drawn[0], flat(0, 100))
        XCTAssertEqual(drawn[1], flat(1, 100))
        XCTAssertEqual(drawn[2], real)
        XCTAssertTrue([PlottedBar]().openingFlatBarsAtThePreviousClose.isEmpty)
    }

    func testCandleBars_theSeriesKeepsTheRawBarsForTheNumbers() throws {
        let points = [100_000_000, 100_000_000, 104_000_000].enumerated().map { index, price in
            MarketChartPoint(
                timestamp: start + Int64(index) * 3_600, priceUsdcMicros: Int64(price),
                openUsdcMicros: Int64(price), highUsdcMicros: Int64(price), lowUsdcMicros: Int64(price))
        }
        let chart = AssetChartSeries(range: .oneWeek, points: points, interval: .h1)

        XCTAssertEqual(chart.bars.map(\.open), [100, 100, 104])
        XCTAssertEqual(chart.candleBars.map(\.open), [100, 100, 100])
        XCTAssertEqual(chart.candleBars.count, chart.bars.count)
        XCTAssertEqual(try XCTUnwrap(chart.describeBar(at: 2)).prefix(10), "Open 104.0")
    }
}
