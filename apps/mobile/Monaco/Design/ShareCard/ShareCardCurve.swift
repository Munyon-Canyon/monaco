import MonacoCore
import SwiftUI

struct ShareCardCurve: View {
    let points: [CurvePoint]
    let tint: Color
    var baseline: Double? = 0

    static let dotRadius: CGFloat = 5
    static let emptyText = "Not enough history yet"

    var body: some View {
        Canvas { context, size in
            guard points.count >= 2 else { return }
            let plot = Self.plot(in: size)
            let xs = points.indices.map { Double(Self.plotPoint(for: $0, of: points, baseline: baseline, in: size).x) }
            let ys = points.indices.map { Double(Self.plotPoint(for: $0, of: points, baseline: baseline, in: size).y) }
            if let baseline, let zero = Self.y(of: baseline, points: points, baseline: baseline, in: size) {
                var rule = Path()
                rule.move(to: CGPoint(x: 0, y: zero))
                rule.addLine(to: CGPoint(x: size.width, y: zero))
                context.stroke(
                    rule, with: .color(MonacoTheme.ShareCard.hairline),
                    style: StrokeStyle(lineWidth: 1, dash: [3, 4]))
            }
            guard let paths = MonacoLineStyle.paths(xs: xs, ys: ys, floorY: plot.maxY) else { return }
            context.fill(paths.fill, with: MonacoLineStyle.fillShading(tint, top: plot.minY, bottom: plot.maxY))
            context.stroke(paths.line, with: .color(tint), style: MonacoLineStyle.stroke)
            if let x = xs.last, let y = ys.last {
                let radius = Self.dotRadius
                context.fill(
                    Path(ellipseIn: CGRect(x: x - radius, y: y - radius, width: radius * 2, height: radius * 2)),
                    with: .color(tint))
            }
        }
        .overlay {
            if points.count < 2 {
                Text(Self.emptyText)
                    .font(.system(size: 13, weight: .medium))
                    .foregroundStyle(MonacoTheme.ShareCard.muted)
            }
        }
        .accessibilityHidden(true)
    }

    static func plot(in size: CGSize) -> CGRect {
        CGRect(origin: .zero, size: size).insetBy(dx: dotRadius, dy: 0)
    }

    static func plotPoint(
        for index: Int, of points: [CurvePoint], baseline: Double? = 0, in size: CGSize
    ) -> CGPoint {
        let plot = plot(in: size)
        let first = points.first?.at.timeIntervalSinceReferenceDate ?? 0
        let last = points.last?.at.timeIntervalSinceReferenceDate ?? 0
        let fraction: Double
        if last > first {
            fraction = (points[index].at.timeIntervalSinceReferenceDate - first) / (last - first)
        } else {
            fraction = points.count > 1 ? Double(index) / Double(points.count - 1) : 0.5
        }
        let y = y(of: Double(points[index].value), points: points, baseline: baseline, in: size) ?? plot.midY
        return CGPoint(x: plot.minX + CGFloat(fraction) * plot.width, y: y)
    }

    private static func y(of value: Double, points: [CurvePoint], baseline: Double?, in size: CGSize) -> CGFloat? {
        var values = points.map { Double($0.value) }
        if let baseline { values.append(baseline) }
        guard let low = values.min(), let high = values.max(), high > low else { return nil }
        let slack = (high - low) * 0.06
        let domain = (low - slack)...(high + slack)
        return CGFloat((domain.upperBound - value) / (domain.upperBound - domain.lowerBound)) * size.height
    }
}
