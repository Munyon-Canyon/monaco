import Foundation

public struct ChartWindow: Equatable, Sendable {
    public static let defaultVisibleBars = 70
    public static let wholeWindowLimit = 100
    public static let minimumBars = 12
    public static let maximumBars = 600
    public static let defaultLead = 10

    public var scrollDate: Date
    public var visibleSeconds: TimeInterval

    public init(scrollDate: Date, visibleSeconds: TimeInterval) {
        self.scrollDate = scrollDate
        self.visibleSeconds = visibleSeconds
    }

    public var endDate: Date { scrollDate.addingTimeInterval(visibleSeconds) }

    public func visibleBarCount(intervalSeconds: Int) -> Int {
        guard intervalSeconds > 0 else { return 0 }
        return Int((visibleSeconds / TimeInterval(intervalSeconds)).rounded())
    }

    public static func initialBarCount(for count: Int) -> Int {
        count > 0 && count <= wholeWindowLimit ? count : defaultVisibleBars
    }

    public static func initial(
        bars: [PlottedBar],
        intervalSeconds: Int,
        visibleBars: Int? = nil
    ) -> ChartWindow {
        let visibleBars = visibleBars ?? initialBarCount(for: bars.count)
        let step = TimeInterval(max(intervalSeconds, 1))
        guard let first = bars.first, let last = bars.last else {
            return ChartWindow(
                scrollDate: Date(timeIntervalSince1970: 0),
                visibleSeconds: step * TimeInterval(visibleBars)
            )
        }
        let shown = max(min(visibleBars, bars.count), minimumBars)
        let scrollDate =
            bars.count < shown
            ? first.date
            : last.date.addingTimeInterval(-step * TimeInterval(shown - 1))
        return ChartWindow(scrollDate: scrollDate, visibleSeconds: step * TimeInterval(shown))
    }

    public func zoomed(
        scale: Double,
        anchorDate: Date,
        intervalSeconds: Int,
        minBars: Int = minimumBars,
        maxBars: Int = maximumBars
    ) -> ChartWindow {
        guard scale.isFinite, scale > 0, visibleSeconds > 0, intervalSeconds > 0 else { return self }
        let step = TimeInterval(intervalSeconds)
        let lower = step * TimeInterval(minBars)
        let upper = max(lower, step * TimeInterval(maxBars))
        let target = min(max(visibleSeconds / scale, lower), upper)
        let fraction = min(max(anchorDate.timeIntervalSince(scrollDate) / visibleSeconds, 0), 1)
        return ChartWindow(
            scrollDate: anchorDate.addingTimeInterval(-fraction * target),
            visibleSeconds: target
        )
    }

    public func clamped(
        to bars: [PlottedBar],
        intervalSeconds: Int,
        minBars: Int = minimumBars,
        maxBars: Int = maximumBars
    ) -> ChartWindow {
        guard let first = bars.first, let last = bars.last, intervalSeconds > 0 else { return self }
        let step = TimeInterval(intervalSeconds)
        let floorBars = min(minBars, max(bars.count, 1))
        let lower = step * TimeInterval(floorBars)
        let upper = max(lower, step * TimeInterval(min(bars.count, maxBars)))
        let visible = min(max(visibleSeconds, lower), upper)
        let earliest = first.date.addingTimeInterval(-step)
        let latestEdge = last.date.addingTimeInterval(step)
        let latestScroll = max(latestEdge.addingTimeInterval(-visible), earliest)
        let scroll = min(max(scrollDate, earliest), latestScroll)
        return ChartWindow(scrollDate: scroll, visibleSeconds: visible)
    }

    public func pinnedToLatest(of bars: [PlottedBar], intervalSeconds: Int) -> ChartWindow {
        guard let last = bars.last else { return self }
        let edge = last.date.addingTimeInterval(TimeInterval(intervalSeconds))
        return ChartWindow(scrollDate: edge.addingTimeInterval(-visibleSeconds), visibleSeconds: visibleSeconds)
    }

    public func visibleRange(of bars: [PlottedBar], slack: TimeInterval = 0) -> Range<Int> {
        let start = Self.firstIndex(in: bars) { $0.date >= scrollDate.addingTimeInterval(-slack) }
        let end = Self.firstIndex(in: bars) { $0.date > endDate.addingTimeInterval(slack) }
        return start..<max(start, end)
    }

    public func valueDomain(
        of bars: [PlottedBar],
        pad: Double = 0.08,
        slack: TimeInterval = 0,
        closesOnly: Bool = false
    ) -> ClosedRange<Double> {
        guard !bars.isEmpty else { return 0...1 }
        let range = visibleRange(of: bars, slack: slack)
        let scope = range.isEmpty ? bars[...] : bars[range]
        var low = scope.map(closesOnly ? \.close : \.low).min() ?? 0
        var high = scope.map(closesOnly ? \.close : \.high).max() ?? 1
        if low > high { swap(&low, &high) }
        let span = high - low
        let padding = span > 0 ? span * pad : max(abs(high) * pad, 0.01)
        return (low - padding)...(high + padding)
    }

    public func isAtLatest(bars: [PlottedBar], intervalSeconds: Int) -> Bool {
        guard let last = bars.last else { return true }
        let step = TimeInterval(intervalSeconds)
        return endDate >= last.date.addingTimeInterval(step / 2)
    }

    public func needsOlder(
        bars: [PlottedBar],
        intervalSeconds: Int,
        hasOlder: Bool,
        lead: Int = defaultLead
    ) -> Bool {
        guard hasOlder, let first = bars.first else { return false }
        return scrollDate <= first.date.addingTimeInterval(TimeInterval(intervalSeconds * lead))
    }

    private static func firstIndex(in bars: [PlottedBar], where predicate: (PlottedBar) -> Bool) -> Int {
        var low = 0
        var high = bars.count
        while low < high {
            let mid = (low + high) / 2
            if predicate(bars[mid]) {
                high = mid
            } else {
                low = mid + 1
            }
        }
        return low
    }
}

public enum ChartAxisLabel {
    public static func text(
        _ date: Date,
        intervalSeconds: Int,
        dayBoundary: Bool = false,
        locale: Locale = .autoupdatingCurrent,
        timeZone: TimeZone = .autoupdatingCurrent
    ) -> String {
        var calendar = Calendar(identifier: .gregorian)
        calendar.locale = locale
        calendar.timeZone = timeZone
        return SharedFormatters.string(
            from: date,
            pattern: .template(template(intervalSeconds: intervalSeconds, dayBoundary: dayBoundary)),
            locale: locale,
            calendar: calendar,
            timeZone: timeZone
        )
    }

    public static func template(intervalSeconds: Int, dayBoundary: Bool) -> String {
        switch intervalSeconds {
        case ...1_800: dayBoundary ? "MMMd" : "jmm"
        case ...3_600: dayBoundary ? "MMMd" : "EEEjmm"
        case ...14_400: "EEEjmm"
        default: "MMMd"
        }
    }

    public static func isDayStart(
        _ date: Date,
        withinSeconds step: TimeInterval,
        timeZone: TimeZone = .autoupdatingCurrent
    ) -> Bool {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = timeZone
        let midnight = calendar.startOfDay(for: date)
        return date.timeIntervalSince(midnight) < max(step, 1)
    }
}

public enum ChartAxisTicks {
    public static let steps: [TimeInterval] = [
        300, 900, 1_800, 3_600, 10_800, 21_600, 43_200, 86_400, 172_800, 604_800, 1_209_600, 2_592_000, 7_776_000,
        15_552_000, 31_536_000,
    ]

    public static let mondayAnchor: TimeInterval = 345_600

    public static func anchor(forBarSeconds seconds: Int, step: TimeInterval) -> TimeInterval {
        seconds >= 604_800 && step.truncatingRemainder(dividingBy: 604_800) == 0 ? mondayAnchor : 0
    }

    public static func step(visibleSeconds: TimeInterval, desired: Int = 6) -> TimeInterval {
        let target = visibleSeconds / TimeInterval(max(desired, 1))
        return steps.first { $0 >= target } ?? steps[steps.count - 1]
    }

    public static func dates(
        from start: Date,
        to end: Date,
        step: TimeInterval,
        limit: Int = 24,
        anchor: TimeInterval = 0,
        timeZone: TimeZone = .autoupdatingCurrent
    ) -> [Date] {
        guard step > 0, end > start else { return [] }
        let offset = TimeInterval(timeZone.secondsFromGMT(for: start))
        var tick = ((start.timeIntervalSince1970 + offset - anchor) / step).rounded(.up) * step + anchor - offset
        var out: [Date] = []
        while tick <= end.timeIntervalSince1970, out.count < limit {
            out.append(Date(timeIntervalSince1970: tick))
            tick += step
        }
        return out
    }

    public static func text(
        _ date: Date,
        step: TimeInterval,
        locale: Locale = .autoupdatingCurrent,
        timeZone: TimeZone = .autoupdatingCurrent
    ) -> String {
        var calendar = Calendar(identifier: .gregorian)
        calendar.locale = locale
        calendar.timeZone = timeZone
        let template: String
        if step >= 15_552_000 {
            template = "MMMyyyy"
        } else if step >= 86_400 || ChartAxisLabel.isDayStart(date, withinSeconds: 1, timeZone: timeZone) {
            template = "MMMd"
        } else {
            template = "jmm"
        }
        return SharedFormatters.string(
            from: date, pattern: .template(template), locale: locale, calendar: calendar, timeZone: timeZone)
    }
}

extension ChartAxisTicks {
    public static func fitting(
        _ dates: [Date],
        scrollDate: Date,
        visibleSeconds: TimeInterval,
        plotWidth: Double,
        labelWidth: (Date) -> Double
    ) -> [Date] {
        guard visibleSeconds > 0, plotWidth > 0 else { return [] }
        return dates.filter { date in
            let center = date.timeIntervalSince(scrollDate) / visibleSeconds * plotWidth
            let half = labelWidth(date) / 2
            return center - half >= 0 && center + half <= plotWidth
        }
    }
}

public enum ChartCrosshairLabel {
    public static func text(
        _ date: Date,
        visibleSeconds: TimeInterval,
        intervalSeconds: Int,
        locale: Locale = .autoupdatingCurrent,
        timeZone: TimeZone = .autoupdatingCurrent
    ) -> String {
        var calendar = Calendar(identifier: .gregorian)
        calendar.locale = locale
        calendar.timeZone = timeZone
        let template: String
        if visibleSeconds <= 129_600 {
            template = "jmm"
        } else if intervalSeconds >= 86_400 {
            template = "MMMd"
        } else {
            template = "MMMdjmm"
        }
        return SharedFormatters.string(
            from: date, pattern: .template(template), locale: locale, calendar: calendar, timeZone: timeZone)
    }
}
