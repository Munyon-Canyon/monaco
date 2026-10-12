import XCTest

@testable import MonacoCore

final class ChartWindowTests: XCTestCase {
    private let epoch = Date(timeIntervalSince1970: 1_790_085_600)
    private let step = 900

    private func bars(_ count: Int, interval: Int? = nil) -> [PlottedBar] {
        let width = interval ?? step
        return (0..<count).map { index in
            let base = 100.0 + TimeInterval(index)
            return PlottedBar(
                date: epoch.addingTimeInterval(TimeInterval(index * width)),
                open: base,
                high: base + 2,
                low: base - 2,
                close: base + 1
            )
        }
    }

    func testInitial_showsTheWholeWindowUpToTheDefaultCapAndThenTheNewestBars() {
        let few = bars(50)
        let fewWindow = ChartWindow.initial(bars: few, intervalSeconds: step)
        XCTAssertEqual(fewWindow.scrollDate, few[0].date)
        XCTAssertEqual(fewWindow.visibleBarCount(intervalSeconds: step), 50)

        let many = bars(900)
        let manyWindow = ChartWindow.initial(bars: many, intervalSeconds: step)
        XCTAssertEqual(manyWindow.visibleBarCount(intervalSeconds: step), ChartWindow.defaultVisibleBars)
        XCTAssertEqual(
            manyWindow.scrollDate,
            many[899].date.addingTimeInterval(-TimeInterval((ChartWindow.defaultVisibleBars - 1) * step)))
    }

    func testInitial_showsTheNewestSixtyBarsAnchoredOnTheLastOne() {
        let series = bars(200)
        let window = ChartWindow.initial(bars: series, intervalSeconds: step, visibleBars: 60)

        XCTAssertEqual(window.visibleSeconds, TimeInterval(60 * step))
        XCTAssertEqual(window.scrollDate, series[199].date.addingTimeInterval(-TimeInterval(59 * step)))
        XCTAssertEqual(window.visibleBarCount(intervalSeconds: step), 60)
    }

    func testInitial_showsEveryBarWhenThereAreFewerThanSixty() {
        let series = bars(30)
        let window = ChartWindow.initial(bars: series, intervalSeconds: step)

        XCTAssertEqual(window.scrollDate, series[0].date)
        XCTAssertEqual(window.visibleSeconds, TimeInterval(30 * step))
    }

    func testInitial_neverShowsFewerThanTheMinimumBars() {
        let series = bars(4)
        let window = ChartWindow.initial(bars: series, intervalSeconds: step)

        XCTAssertEqual(window.scrollDate, series[0].date)
        XCTAssertEqual(window.visibleBarCount(intervalSeconds: step), ChartWindow.minimumBars)
    }

    func testInitial_survivesAnEmptySeries() {
        let window = ChartWindow.initial(bars: [], intervalSeconds: step)

        XCTAssertEqual(window.visibleBarCount(intervalSeconds: step), ChartWindow.defaultVisibleBars)
        XCTAssertEqual(window.valueDomain(of: []), 0...1)
        XCTAssertTrue(window.isAtLatest(bars: [], intervalSeconds: step))
    }

    func testZoomed_keepsTheAnchorAtTheSameScreenFraction() {
        let window = ChartWindow(scrollDate: epoch, visibleSeconds: TimeInterval(60 * step))
        let anchor = epoch.addingTimeInterval(TimeInterval(15 * step))

        let zoomed = window.zoomed(scale: 2, anchorDate: anchor, intervalSeconds: step)

        XCTAssertEqual(zoomed.visibleSeconds, TimeInterval(30 * step), accuracy: 0.001)
        let before = anchor.timeIntervalSince(window.scrollDate) / window.visibleSeconds
        let after = anchor.timeIntervalSince(zoomed.scrollDate) / zoomed.visibleSeconds
        XCTAssertEqual(before, after, accuracy: 1e-9)
    }

    func testZoomed_pinchOutShowsMoreBars() {
        let window = ChartWindow(scrollDate: epoch, visibleSeconds: TimeInterval(40 * step))
        let anchor = epoch.addingTimeInterval(TimeInterval(20 * step))
        let zoomed = window.zoomed(scale: 0.5, anchorDate: anchor, intervalSeconds: step)

        XCTAssertEqual(zoomed.visibleBarCount(intervalSeconds: step), 80)
    }

    func testZoomed_clampsToTheBarLimits() {
        let window = ChartWindow(scrollDate: epoch, visibleSeconds: TimeInterval(60 * step))
        let anchor = epoch.addingTimeInterval(TimeInterval(30 * step))

        XCTAssertEqual(
            window.zoomed(scale: 100, anchorDate: anchor, intervalSeconds: step).visibleBarCount(intervalSeconds: step),
            12
        )
        let wide = window.zoomed(scale: 0.001, anchorDate: anchor, intervalSeconds: step)
        XCTAssertEqual(wide.visibleBarCount(intervalSeconds: step), ChartWindow.maximumBars)
        XCTAssertEqual(
            window.zoomed(scale: 0.001, anchorDate: anchor, intervalSeconds: step, maxBars: 90)
                .visibleBarCount(intervalSeconds: step),
            90
        )
    }

    func testZoomed_ignoresANonsenseScale() {
        let window = ChartWindow(scrollDate: epoch, visibleSeconds: TimeInterval(60 * step))

        for scale in [0.0, -1, .nan, .infinity] {
            XCTAssertEqual(window.zoomed(scale: scale, anchorDate: epoch, intervalSeconds: step), window)
        }
    }

    func testZoomed_anchorOutsideTheWindowPinsToTheNearestEdge() {
        let window = ChartWindow(scrollDate: epoch, visibleSeconds: TimeInterval(60 * step))
        let zoomed = window.zoomed(scale: 2, anchorDate: epoch.addingTimeInterval(-10_000), intervalSeconds: step)

        XCTAssertEqual(zoomed.scrollDate, epoch.addingTimeInterval(-10_000))
    }

    func testClamped_neverScrollsPastTheFirstBar() {
        let series = bars(200)
        let early = epoch.addingTimeInterval(-1_000_000)
        let window = ChartWindow(scrollDate: early, visibleSeconds: TimeInterval(60 * step))

        let clamped = window.clamped(to: series, intervalSeconds: step)

        XCTAssertEqual(clamped.scrollDate, epoch.addingTimeInterval(-TimeInterval(step)))
    }

    func testClamped_neverScrollsBeyondTheLastBar() {
        let series = bars(200)
        let later = epoch.addingTimeInterval(10_000_000)
        let window = ChartWindow(scrollDate: later, visibleSeconds: TimeInterval(60 * step))

        let clamped = window.clamped(to: series, intervalSeconds: step)

        XCTAssertEqual(clamped.endDate, series[199].date.addingTimeInterval(TimeInterval(step)))
        XCTAssertTrue(clamped.isAtLatest(bars: series, intervalSeconds: step))
    }

    func testClamped_leavesAWindowInsideTheSeriesAlone() {
        let series = bars(200)
        let window = ChartWindow(
            scrollDate: epoch.addingTimeInterval(TimeInterval(50 * step)),
            visibleSeconds: TimeInterval(40 * step)
        )

        XCTAssertEqual(window.clamped(to: series, intervalSeconds: step), window)
    }

    func testClamped_limitsTheVisibleBarsToTheSeriesAndTheCap() {
        let few = bars(30)
        let tooWide = ChartWindow(scrollDate: epoch, visibleSeconds: TimeInterval(2_000 * step))
        XCTAssertEqual(tooWide.clamped(to: few, intervalSeconds: step).visibleBarCount(intervalSeconds: step), 30)

        let many = bars(900)
        let limited = tooWide.clamped(to: many, intervalSeconds: step, maxBars: 300)
        XCTAssertEqual(limited.visibleBarCount(intervalSeconds: step), 300)
        let capped = tooWide.clamped(to: many, intervalSeconds: step)
        XCTAssertEqual(capped.visibleBarCount(intervalSeconds: step), ChartWindow.maximumBars)
    }

    func testVisibleRange_isTheBarsInsideTheWindow() {
        let series = bars(100)
        let window = ChartWindow(
            scrollDate: epoch.addingTimeInterval(TimeInterval(10 * step)),
            visibleSeconds: TimeInterval(20 * step)
        )

        XCTAssertEqual(window.visibleRange(of: series), 10..<31)
        XCTAssertEqual(window.visibleRange(of: series, slack: TimeInterval(step)), 9..<32)
    }

    func testVisibleRange_isEmptyOutsideTheSeries() {
        let series = bars(10)
        let window = ChartWindow(
            scrollDate: epoch.addingTimeInterval(TimeInterval(50 * step)),
            visibleSeconds: TimeInterval(20 * step)
        )

        XCTAssertTrue(window.visibleRange(of: series).isEmpty)
        XCTAssertTrue(window.visibleRange(of: []).isEmpty)
    }

    func testValueDomain_isTheVisibleLowAndHighPlusEightPercent() {
        let series = bars(100)
        let window = ChartWindow(
            scrollDate: epoch.addingTimeInterval(TimeInterval(10 * step)),
            visibleSeconds: TimeInterval(10 * step)
        )

        let domain = window.valueDomain(of: series)

        XCTAssertEqual(domain.lowerBound, 108 - 14 * 0.08, accuracy: 1e-9)
        XCTAssertEqual(domain.upperBound, 122 + 14 * 0.08, accuracy: 1e-9)
    }

    func testValueDomain_followsThePanBecauseItReadsOnlyVisibleBars() {
        let series = bars(100)
        let early = ChartWindow(scrollDate: epoch, visibleSeconds: TimeInterval(10 * step))
        let lateStart = epoch.addingTimeInterval(TimeInterval(80 * step))
        let late = ChartWindow(scrollDate: lateStart, visibleSeconds: TimeInterval(10 * step))

        XCTAssertLessThan(early.valueDomain(of: series).upperBound, late.valueDomain(of: series).lowerBound + 20)
        XCTAssertLessThan(early.valueDomain(of: series).upperBound, late.valueDomain(of: series).upperBound)
    }

    func testValueDomain_fallsBackToTheWholeSeriesWhenTheWindowIsEmpty() {
        let series = bars(10)
        let window = ChartWindow(scrollDate: epoch.addingTimeInterval(1_000_000), visibleSeconds: 900)

        let domain = window.valueDomain(of: series)

        XCTAssertEqual(domain.lowerBound, 98 - 13 * 0.08, accuracy: 1e-9)
        XCTAssertEqual(domain.upperBound, 111 + 13 * 0.08, accuracy: 1e-9)
    }

    func testValueDomain_padsAFlatSeries() {
        let flat = [PlottedBar(date: epoch, open: 50, high: 50, low: 50, close: 50)]
        let window = ChartWindow(scrollDate: epoch, visibleSeconds: 900)

        let domain = window.valueDomain(of: flat)

        XCTAssertLessThan(domain.lowerBound, 50)
        XCTAssertGreaterThan(domain.upperBound, 50)
    }

    func testIsAtLatest_isTrueWithinHalfABarOfTheEnd() {
        let series = bars(200)
        let initial = ChartWindow.initial(bars: series, intervalSeconds: step)
        let pinned = initial.pinnedToLatest(of: series, intervalSeconds: step)

        XCTAssertTrue(pinned.isAtLatest(bars: series, intervalSeconds: step))

        var nudged = pinned
        nudged.scrollDate = pinned.scrollDate.addingTimeInterval(-TimeInterval(step) / 2 + 1)
        XCTAssertTrue(nudged.isAtLatest(bars: series, intervalSeconds: step))

        var scrolled = pinned
        scrolled.scrollDate = pinned.scrollDate.addingTimeInterval(-TimeInterval(step * 3))
        XCTAssertFalse(scrolled.isAtLatest(bars: series, intervalSeconds: step))
    }

    func testPinnedToLatest_putsTheLastBarOneBarFromTheRightEdge() {
        let series = bars(200)
        let window = ChartWindow(scrollDate: epoch, visibleSeconds: TimeInterval(40 * step)).pinnedToLatest(
            of: series,
            intervalSeconds: step
        )

        XCTAssertEqual(window.endDate, series[199].date.addingTimeInterval(TimeInterval(step)))
        XCTAssertEqual(window.visibleSeconds, TimeInterval(40 * step))
    }

    func testNeedsOlder_isTrueOnlyNearTheFirstBarAndOnlyWhenOlderBarsExist() {
        let series = bars(200)
        let near = ChartWindow(
            scrollDate: series[0].date.addingTimeInterval(TimeInterval(9 * step)),
            visibleSeconds: TimeInterval(60 * step)
        )
        let far = ChartWindow(
            scrollDate: series[0].date.addingTimeInterval(TimeInterval(11 * step)),
            visibleSeconds: TimeInterval(60 * step)
        )

        XCTAssertTrue(near.needsOlder(bars: series, intervalSeconds: step, hasOlder: true))
        XCTAssertFalse(far.needsOlder(bars: series, intervalSeconds: step, hasOlder: true))
        XCTAssertFalse(near.needsOlder(bars: series, intervalSeconds: step, hasOlder: false))
        XCTAssertFalse(near.needsOlder(bars: [], intervalSeconds: step, hasOlder: true))
    }

    func testValueDomain_closesOnlyIgnoresTheWicks() {
        let series = bars(100)
        let window = ChartWindow(
            scrollDate: epoch.addingTimeInterval(TimeInterval(10 * step)),
            visibleSeconds: TimeInterval(10 * step)
        )

        let full = window.valueDomain(of: series)
        let closes = window.valueDomain(of: series, closesOnly: true)

        XCTAssertGreaterThan(closes.lowerBound, full.lowerBound)
        XCTAssertLessThan(closes.upperBound, full.upperBound)
        XCTAssertEqual(closes.lowerBound, 111 - 10 * 0.08, accuracy: 1e-9)
        XCTAssertEqual(closes.upperBound, 121 + 10 * 0.08, accuracy: 1e-9)
    }

    func testDefaultVisibleBars_isAboutSeventy() {
        XCTAssertEqual(ChartWindow.defaultVisibleBars, 70)
    }

    func testInitialBarCount_isTheWholeWindowUpToAHundredBarsThenTheNewestSeventy() {
        XCTAssertEqual(ChartWindow.initialBarCount(for: 13), 13)
        XCTAssertEqual(ChartWindow.initialBarCount(for: 100), 100)
        XCTAssertEqual(ChartWindow.initialBarCount(for: 101), 70)
        XCTAssertEqual(ChartWindow.initialBarCount(for: 2_000), 70)
        XCTAssertEqual(ChartWindow.initialBarCount(for: 0), 70)
    }

    func testInitial_aHundredBarWindowOpensWhole() {
        let series = bars(100)
        let window = ChartWindow.initial(bars: series, intervalSeconds: step)

        XCTAssertEqual(window.scrollDate, series[0].date)
        XCTAssertEqual(window.visibleBarCount(intervalSeconds: step), 100)
    }
}
