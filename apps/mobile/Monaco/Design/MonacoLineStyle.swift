import MonacoCore
import SwiftUI

enum MonacoLineStyle {
    static let stroke = StrokeStyle(lineWidth: 2.75, lineCap: .round, lineJoin: .round)

    static func fillGradient(_ tint: Color) -> Gradient {
        Gradient(colors: [tint.opacity(0.25), tint.opacity(0)])
    }

    static func fillShading(_ tint: Color, top: CGFloat, bottom: CGFloat) -> GraphicsContext.Shading {
        .linearGradient(
            fillGradient(tint), startPoint: CGPoint(x: 0, y: top), endPoint: CGPoint(x: 0, y: bottom))
    }

    static func paths(xs: [Double], ys: [Double], floorY: CGFloat) -> (line: Path, fill: Path)? {
        let segments = ChartCurve.monotone(xs: xs, ys: ys)
        guard let firstX = xs.first, let firstY = ys.first, let lastX = xs.last, !segments.isEmpty else { return nil }
        var line = Path()
        line.move(to: CGPoint(x: firstX, y: firstY))
        for segment in segments {
            line.addCurve(
                to: CGPoint(x: segment.end.x, y: segment.end.y),
                control1: CGPoint(x: segment.control1.x, y: segment.control1.y),
                control2: CGPoint(x: segment.control2.x, y: segment.control2.y))
        }
        var fill = line
        fill.addLine(to: CGPoint(x: lastX, y: floorY))
        fill.addLine(to: CGPoint(x: firstX, y: floorY))
        fill.closeSubpath()
        return (line, fill)
    }
}
