import MonacoCore
import SwiftUI

struct FShareChart: View {
    let chart: ShareChart
    var tone: ShareFigureTone = .gain
    var progress: Double = 1
    var scale: CGFloat = 1

    static let discSize: CGFloat = 16
    static let sealSize: CGFloat = 20
    static let ringWidth: CGFloat = 2

    private var lineColor: Color { tone == .gain ? MonacoTheme.profit : MonacoTheme.loss }

    var body: some View {
        GeometryReader { proxy in
            let layout = Layout(chart: chart, size: proxy.size, scale: scale)
            let cutoff = layout.cutoffX(progress: progress)
            ZStack(alignment: .topLeading) {
                Canvas { context, _ in
                    let baseline = Double(layout.size.height)
                    guard let line = MonacoLineStyle.line(xs: layout.xs, ys: layout.ys),
                        let area = MonacoLineStyle.area(xs: layout.xs, ys: layout.ys, baseline: baseline)
                    else { return }
                    context.clip(to: Path(CGRect(x: 0, y: 0, width: cutoff, height: layout.size.height)))
                    context.fill(area, with: .color(lineColor.opacity(MonacoLineStyle.fillOpacity)))
                    context.stroke(
                        line, with: .color(lineColor),
                        style: StrokeStyle(lineWidth: 1.7 * scale, lineCap: .round, lineJoin: .round))
                }
                ForEach(Array(layout.discs.enumerated()), id: \.offset) { _, disc in
                    if disc.center.x <= cutoff + 0.5 {
                        FShareDisc(kind: disc.marker.kind, scale: scale)
                            .position(disc.center)
                        if let callout = disc.callout {
                            FShareCallout(kind: disc.marker.kind, text: callout.text, scale: scale)
                                .position(callout.center)
                        }
                    }
                }
            }
        }
        .accessibilityHidden(true)
    }

    struct Disc {
        let marker: ShareMarker
        let center: CGPoint
        let callout: (text: String, center: CGPoint)?
    }

    struct Layout {
        let size: CGSize
        let xs: [Double]
        let ys: [Double]
        let discs: [Disc]

        init(chart: ShareChart, size: CGSize, scale: CGFloat) {
            self.size = size
            let radius = FShareChart.discSize * scale / 2
            let calloutHeight = 22 * scale
            let showsCallouts = size.height >= 100 * scale
            let points = chart.points
            let times = points.map(\.time.timeIntervalSince1970) + chart.markers.map(\.time.timeIntervalSince1970)
            let prices = points.map(\.priceMicros) + chart.markers.map(\.priceMicros)
            guard let minT = times.min(), let maxT = times.max(), let minP = prices.min(), let maxP = prices.max(),
                points.count >= 2
            else {
                xs = []
                ys = []
                discs = []
                return
            }
            let top = (showsCallouts ? calloutHeight : 0) + radius + 4 * scale
            let bottom = top
            let spanT = max(maxT - minT, 1)
            let spanP = Double(max(maxP - minP, 1))
            let plotHeight = max(size.height - top - bottom, 1)
            func x(_ time: TimeInterval) -> Double { (time - minT) / spanT * size.width }
            func y(_ price: Int64) -> Double { top + (1 - Double(price - minP) / spanP) * plotHeight }
            xs = points.map { x($0.time.timeIntervalSince1970) }
            ys = points.map { y($0.priceMicros) }
            let hasSeal = chart.markers.contains { $0.kind == .seal }
            var placed: [CGPoint] = []
            var built: [Disc] = []
            for marker in chart.markers {
                var center = CGPoint(
                    x: min(max(x(marker.time.timeIntervalSince1970), radius), size.width - radius),
                    y: y(marker.priceMicros))
                let overlaps = { (other: CGPoint) in
                    abs(other.x - center.x) < radius * 2 && abs(other.y - center.y) < radius * 2
                }
                while placed.contains(where: overlaps), center.y > radius {
                    center.y -= radius * 1.5
                }
                placed.append(center)
                built.append(
                    Disc(
                        marker: marker, center: center,
                        callout: (showsCallouts ? marker.callout : nil).map {
                            (
                                $0,
                                Self.calloutCenter(
                                    for: marker.kind, text: $0, disc: center, radius: radius, height: calloutHeight,
                                    scale: scale, size: size, fillsAbove: hasSeal)
                            )
                        }))
            }
            discs = built
        }

        func cutoffX(progress: Double) -> CGFloat {
            guard progress < 1, let first = xs.first, let last = xs.last else { return size.width }
            return CGFloat(first + (last - first) * max(progress, 0))
        }

        private static func calloutCenter(
            for kind: ShareMarker.Kind, text: String, disc: CGPoint, radius: CGFloat, height: CGFloat, scale: CGFloat,
            size: CGSize, fillsAbove: Bool
        ) -> CGPoint {
            let above = kind == .sell || (fillsAbove && kind != .seal)
            var y = above ? disc.y - radius - height / 2 - 2 : disc.y + radius + height / 2 + 2
            if y - height / 2 < 0 { y = disc.y + radius + height / 2 + 2 }
            if y + height / 2 > size.height { y = disc.y - radius - height / 2 - 2 }
            let half = (CGFloat(text.count) * 6.4 + 24) * scale / 2
            return CGPoint(x: min(max(disc.x, half), size.width - half), y: y)
        }
    }
}

private struct FShareDisc: View {
    let kind: ShareMarker.Kind
    let scale: CGFloat

    var body: some View {
        let size = FShareChart.discSize * scale
        let ring = size + FShareChart.ringWidth * 2 * scale
        ZStack {
            if kind == .seal {
                Circle().fill(Color.black)
                    .frame(width: FShareChart.sealSize * scale, height: FShareChart.sealSize * scale)
                Image(systemName: "checkmark.seal.fill")
                    .font(.system(size: FShareChart.sealSize * scale))
                    .foregroundStyle(MonacoTheme.profit)
            } else {
                Circle().fill(Color.black)
                    .frame(width: ring, height: ring)
                Circle().fill(kind == .sell ? MonacoTheme.loss : MonacoTheme.profit)
                    .frame(width: size, height: size)
                Text(kind == .sell ? "S" : "B")
                    .font(.system(size: 10 * scale, weight: .heavy))
                    .foregroundStyle(kind == .sell ? Color.white : Color.black)
            }
        }
    }
}

private struct FShareCallout: View {
    let kind: ShareMarker.Kind
    let text: String
    let scale: CGFloat

    private var glyph: String {
        switch kind {
        case .buy: "B"
        case .sell: "S"
        case .seal: "\u{2713}"
        }
    }

    var body: some View {
        HStack(spacing: 4 * scale) {
            Text(glyph)
                .foregroundStyle(kind == .sell ? MonacoTheme.loss : MonacoTheme.profit)
            Text(text).foregroundStyle(Color.white)
        }
        .font(.system(size: 13 * scale, weight: .semibold).monospacedDigit())
        .lineLimit(1)
        .fixedSize()
        .padding(.horizontal, 6 * scale)
        .frame(height: 22 * scale)
        .background(Color.black.opacity(0.78), in: RoundedRectangle(cornerRadius: 4 * scale, style: .continuous))
    }
}
