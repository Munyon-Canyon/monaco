import MonacoCore
import SwiftUI

struct CurveScrubChart: View {
    let curve: ValueCurve
    let range: LeaderboardRange
    var height: CGFloat = 160
    var onInk = false
    @Binding var selection: Int?
    let identifier: String

    private static let unitsPerDollar = 1_000_000.0

    private var tint: Color {
        switch curve.direction {
        case .up: MonacoTheme.profitVivid
        case .down: MonacoTheme.lossVivid
        case .flat: onInk ? MonacoTheme.onHeroMuted : MonacoTheme.muted
        }
    }

    var body: some View {
        MonacoScrubChart(
            points: curve.points.map { .init(date: $0.at, value: Double($0.value) / Self.unitsPerDollar) },
            tint: tint,
            baseline: nil,
            height: height,
            drawOnKey: range.rawValue,
            selection: $selection,
            summary: "\(range.accessibilityName) value history",
            nearestIndex: { curve.nearestIndex(to: $0) },
            describePoint: { index in
                curve.readout(at: index).map { "\($0.value), \($0.pnl)" } ?? ""
            },
            accessibilityIdentifier: identifier
        )
    }
}

struct CurveReadoutLine: View {
    let readout: ValueCurve.Readout?
    var onInk = false

    var body: some View {
        Text(text)
            .font(MonacoTheme.Typo.dataCaption)
            .foregroundStyle(onInk ? MonacoTheme.onHeroMuted : MonacoTheme.muted)
            .lineLimit(1)
            .minimumScaleFactor(0.8)
            .opacity(readout == nil ? 0 : 1)
            .accessibilityHidden(readout == nil)
    }

    private var text: String {
        guard let readout else { return " " }
        return "\(readout.value) · \(readout.pnl) · \(readout.at.formatted(.dateTime.month().day().hour().minute()))"
    }
}
