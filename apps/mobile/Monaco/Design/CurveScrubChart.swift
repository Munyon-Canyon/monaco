import MonacoCore
import SwiftUI

struct CurveScrubChart: View {
    static let defaultHeight: CGFloat = 170
    static let smallHeight: CGFloat = 148

    let curve: ValueCurve
    let plot: ValueCurve.Plot
    let range: LeaderboardRange
    var height: CGFloat = CurveScrubChart.defaultHeight
    var showsDateRange = true
    @Binding var selection: Int?
    let identifier: String

    private var points: [CurvePoint] {
        let plotted = curve.plotted(plot)
        guard plotted.count == 1, let only = plotted.first else { return plotted }
        return [CurvePoint(at: only.at.addingTimeInterval(-1), value: only.value), only]
    }

    private var tint: Color {
        switch curve.change(plot)?.direction ?? .flat {
        case .up: MonacoTheme.profitVivid
        case .down: MonacoTheme.lossVivid
        case .flat: MonacoTheme.secondaryText
        }
    }

    private static func units(_ micros: Int64) -> Double {
        NSDecimalNumber(value: micros).dividing(by: NSDecimalNumber(value: 1_000_000)).doubleValue
    }

    static func tooltipAmount(_ change: ValueCurve.Change?) -> String? {
        change?.amountText ?? change?.percentText
    }

    private func change(at index: Int) -> ValueCurve.Change? {
        curve.change(plot, to: min(index, curve.plotted(plot).count - 1))
    }

    var body: some View {
        let points = points
        let bars = points.map { point -> PlottedBar in
            let value = Self.units(point.value)
            return PlottedBar(date: point.at, open: value, high: value, low: value, close: value)
        }
        let domain = ValueCurve.fittedDomain(points)
        MonacoCandleChart(
            bars: bars,
            intervalSeconds: curve.slotSeconds,
            style: .line,
            tint: tint,
            lastPrice: nil,
            isLive: range == .oneHour || range == .oneDay,
            fixedDomain: Self.units(domain.lowerBound)...Self.units(domain.upperBound),
            showsLastPriceRule: false,
            showsEndDot: true,
            selectionStyle: .tooltipPill,
            showsDateRange: showsDateRange,
            resetKey: range.rawValue,
            hasOlder: false,
            initialVisibleBars: bars.count,
            tooltipAmount: { Self.tooltipAmount(change(at: $0)) },
            gutter: 0,
            height: height,
            selection: $selection,
            onNeedOlder: {},
            summary: "\(range.accessibilityName) performance history",
            nearestIndex: { date in
                points.indices.min {
                    abs(points[$0].at.timeIntervalSince(date)) < abs(points[$1].at.timeIntervalSince(date))
                }
            },
            describeBar: { index in
                let moved = change(at: index)
                return [moved?.amountText, moved?.percentText].compactMap { $0 }.joined(separator: " ")
            },
            accessibilityIdentifier: identifier
        )
    }
}

struct CurveScrubChartFrame<Content: View>: View {
    var height: CGFloat = CurveScrubChart.defaultHeight
    var showsDateRange = true
    @ViewBuilder var content: Content

    var body: some View {
        VStack(spacing: 0) {
            content.frame(height: height)
            if showsDateRange {
                MonacoChartDateRow(first: " ", last: " ").hidden()
            }
        }
    }
}

struct CurveChangeLine: View {
    let curve: ValueCurve?
    let plot: ValueCurve.Plot
    var range: LeaderboardRange = .oneDay
    var selection: Int?

    var body: some View {
        Text(text)
            .font(MonacoTheme.Typo.dataCaption)
            .foregroundStyle(color)
            .lineLimit(1)
            .minimumScaleFactor(0.8)
            .accessibilityIdentifier("curve-change-line")
    }

    private var change: ValueCurve.Change? {
        guard let curve else { return nil }
        guard let selection else { return curve.change(plot) }
        return curve.change(plot, to: min(selection, max(curve.plotted(plot).count - 1, 0)))
    }

    private var color: Color {
        switch change?.direction ?? .flat {
        case .up: MonacoTheme.profitVivid
        case .down: MonacoTheme.lossVivid
        case .flat: MonacoTheme.muted
        }
    }

    private var text: String {
        let date = selection.flatMap { selected -> Date? in
            guard let points = curve?.plotted(plot), !points.isEmpty else { return nil }
            return points[min(selected, points.count - 1)].at
        }
        return Self.text(change: change, range: range, at: date)
    }

    static func text(
        change: ValueCurve.Change?,
        range: LeaderboardRange,
        at date: Date? = nil,
        locale: Locale = .autoupdatingCurrent,
        timeZone: TimeZone = .autoupdatingCurrent
    ) -> String {
        guard let change else { return " " }
        var parts = [change.amountText, change.percentText.map { change.amountText == nil ? $0 : "(\($0))" }]
            .compactMap { $0 }
        if parts.isEmpty { return " " }
        if let date {
            let stamp = ChartScrubLabel.caption(
                for: date, range: range.scrubLabelRange, locale: locale, timeZone: timeZone)
            parts.append("· \(stamp)")
        } else {
            parts.append(range.accessibilityName)
        }
        return parts.joined(separator: " ")
    }
}
