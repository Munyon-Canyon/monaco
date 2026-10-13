import MonacoCore
import SwiftUI

struct CurveScrubChart: View {
    let curve: ValueCurve
    let plot: ValueCurve.Plot
    let range: LeaderboardRange
    var height: CGFloat = 160
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
        case .flat: MonacoTheme.tertiaryText
        }
    }

    private static func units(_ micros: Int64) -> Double {
        NSDecimalNumber(value: micros).dividing(by: NSDecimalNumber(value: 1_000_000)).doubleValue
    }

    private func text(_ value: Double) -> String {
        switch plot {
        case .pnl: UsdAmountFormatter.format(signedMicros: Int64((value * 1_000_000).rounded()))
        case .nav: value.formatted(.number.precision(.fractionLength(4)))
        }
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
            lastPrice: bars.last?.close,
            isLive: range == .oneHour || range == .oneDay,
            fixedDomain: Self.units(domain.lowerBound)...Self.units(domain.upperBound),
            resetKey: range.rawValue,
            hasOlder: false,
            initialVisibleBars: bars.count,
            valueText: text,
            gutter: 88,
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
                let at = min(index, curve.plotted(plot).count - 1)
                let change = curve.change(plot, to: at)
                return [change?.amountText, change?.percentText].compactMap { $0 }.joined(separator: " ")
            },
            accessibilityIdentifier: identifier
        )
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
