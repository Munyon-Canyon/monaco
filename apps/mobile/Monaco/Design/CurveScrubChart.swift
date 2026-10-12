import MonacoCore
import SwiftUI

struct CurveScrubChart: View {
    let curve: ValueCurve
    let range: LeaderboardRange
    var height: CGFloat = 160
    @Binding var selection: Int?
    let identifier: String

    private var tint: Color {
        switch curve.direction {
        case .up: MonacoTheme.profitVivid
        case .down: MonacoTheme.lossVivid
        case .flat: MonacoTheme.tertiaryText
        }
    }

    var body: some View {
        let bars = curve.chartBars
        MonacoCandleChart(
            bars: bars,
            intervalSeconds: curve.slotSeconds,
            style: .line,
            tint: tint,
            lastPrice: bars.last?.close,
            isLive: range == .oneHour || range == .oneDay,
            resetKey: range.rawValue,
            hasOlder: false,
            initialVisibleBars: bars.count,
            valueText: { UsdAmountFormatter.format(micros: Int64(($0 * 1_000_000).rounded())) },
            gutter: 88,
            height: height,
            selection: $selection,
            onNeedOlder: {},
            summary: "\(range.accessibilityName) value history",
            nearestIndex: { curve.nearestIndex(to: $0) },
            describeBar: { index in
                curve.readout(at: index).map { "\($0.value), \($0.pnl)" } ?? ""
            },
            accessibilityIdentifier: identifier
        )
    }
}

struct CurveReadoutLine: View {
    let readout: ValueCurve.Readout?
    var range: LeaderboardRange = .oneDay

    var body: some View {
        Text(text)
            .font(MonacoTheme.Typo.dataCaption)
            .foregroundStyle(MonacoTheme.muted)
            .lineLimit(1)
            .minimumScaleFactor(0.8)
            .opacity(readout == nil ? 0 : 1)
            .accessibilityHidden(readout == nil)
    }

    private var text: String { Self.text(for: readout, range: range) }

    static func text(
        for readout: ValueCurve.Readout?,
        range: LeaderboardRange,
        locale: Locale = .autoupdatingCurrent,
        timeZone: TimeZone = .autoupdatingCurrent
    ) -> String {
        guard let readout else { return " " }
        let date = ChartScrubLabel.caption(
            for: readout.at, range: range.scrubLabelRange, locale: locale, timeZone: timeZone)
        return "\(readout.value) · \(readout.pnl) · \(date)"
    }
}
