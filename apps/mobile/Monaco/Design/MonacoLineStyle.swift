import MonacoCore
import SwiftUI

enum MonacoLineStyle {
    static let stroke = StrokeStyle(lineWidth: 1.7, lineCap: .round, lineJoin: .round)

    static let endDotRadius: CGFloat = 3.5

    static func line(xs: [Double], ys: [Double]) -> Path? {
        let segments = ChartCurve.monotone(xs: xs, ys: ys)
        guard let firstX = xs.first, let firstY = ys.first, !segments.isEmpty else { return nil }
        var line = Path()
        line.move(to: CGPoint(x: firstX, y: firstY))
        for segment in segments {
            line.addCurve(
                to: CGPoint(x: segment.end.x, y: segment.end.y),
                control1: CGPoint(x: segment.control1.x, y: segment.control1.y),
                control2: CGPoint(x: segment.control2.x, y: segment.control2.y))
        }
        return line
    }
}

enum ChartMetrics {
    static let tooltipRadius: CGFloat = 9
    static let quietHeight: CGFloat = 104
    static let crosshairWidth: CGFloat = 1
}

enum MonacoChartSelectionStyle {
    case axisTag
    case tooltipPill
}
