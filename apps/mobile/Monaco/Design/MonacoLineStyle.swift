import MonacoCore
import SwiftUI

enum MonacoLineStyle {
    static let stroke = StrokeStyle(lineWidth: 1.7, lineCap: .round, lineJoin: .round)

    static let endDotRadius: CGFloat = 3.5

    static let fillOpacity = 0.10

    static func line(xs: [Double], ys: [Double]) -> Path? {
        let segments = ChartCurve.monotone(xs: xs, ys: ys)
        guard let firstX = xs.first, let firstY = ys.first, !segments.isEmpty else { return nil }
        var line = Path()
        line.move(to: CGPoint(x: firstX, y: firstY))
        addCurve(segments, to: &line)
        return line
    }

    static func area(xs: [Double], ys: [Double], baseline: Double) -> Path? {
        let segments = ChartCurve.monotone(xs: xs, ys: ys)
        guard let firstX = xs.first, let firstY = ys.first, let lastX = xs.last, !segments.isEmpty else { return nil }
        var area = Path()
        area.move(to: CGPoint(x: firstX, y: baseline))
        area.addLine(to: CGPoint(x: firstX, y: firstY))
        addCurve(segments, to: &area)
        area.addLine(to: CGPoint(x: lastX, y: baseline))
        area.closeSubpath()
        return area
    }

    private static func addCurve(_ segments: [ChartCurve.Segment], to path: inout Path) {
        for segment in segments {
            path.addCurve(
                to: CGPoint(x: segment.end.x, y: segment.end.y),
                control1: CGPoint(x: segment.control1.x, y: segment.control1.y),
                control2: CGPoint(x: segment.control2.x, y: segment.control2.y))
        }
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
