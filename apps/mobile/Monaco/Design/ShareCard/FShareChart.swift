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

    private var lineColor: Color { tone == .loss ? MonacoTheme.loss : MonacoTheme.profit }

    var body: some View {
        GeometryReader { proxy in
            let layout = Layout(chart: chart, size: proxy.size, scale: scale)
            let cutoff = layout.cutoffX(progress: progress)
            ZStack(alignment: .topLeading) {
                Canvas { context, _ in
                    let baseline = Double(layout.size.height)
                    guard let line = Self.straightLine(xs: layout.xs, ys: layout.ys),
                        let area = Self.straightArea(xs: layout.xs, ys: layout.ys, baseline: baseline)
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

    private static func straightLine(xs: [Double], ys: [Double]) -> Path? {
        guard let firstX = xs.first, let firstY = ys.first, xs.count == ys.count, xs.count >= 2 else { return nil }
        var line = Path()
        line.move(to: CGPoint(x: firstX, y: firstY))
        for index in 1..<xs.count { line.addLine(to: CGPoint(x: xs[index], y: ys[index])) }
        return line
    }

    private static func straightArea(xs: [Double], ys: [Double], baseline: Double) -> Path? {
        guard var area = straightLine(xs: xs, ys: ys), let firstX = xs.first, let lastX = xs.last else { return nil }
        area.addLine(to: CGPoint(x: lastX, y: baseline))
        area.addLine(to: CGPoint(x: firstX, y: baseline))
        area.closeSubpath()
        return area
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
            let showsCallouts = size.height >= 48 * scale
            let roomy = size.height >= 100 * scale
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
            let top = (roomy ? calloutHeight : 0) + radius + 4 * scale
            let bottom = roomy ? top : 4 * scale
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
            discs = Self.stacked(built, height: calloutHeight, scale: scale, size: size)
        }

        private static func stacked(_ discs: [Disc], height: CGFloat, scale: CGFloat, size: CGSize) -> [Disc] {
            var result = discs
            let gap = 2 * scale
            func rect(_ callout: (text: String, center: CGPoint)) -> CGRect {
                let width = (CGFloat(callout.text.count) * 6.4 + 24) * scale
                return CGRect(
                    x: callout.center.x - width / 2, y: callout.center.y - height / 2, width: width, height: height)
            }
            let order = result.indices.sorted { result[$0].marker.time < result[$1].marker.time }
            for (position, index) in order.enumerated() {
                guard let later = result[index].callout else { continue }
                for earlierIndex in order[..<position] {
                    guard let earlier = result[earlierIndex].callout,
                        rect(earlier).intersects(rect(later))
                    else { continue }
                    let current = result[index].callout ?? later
                    let lifted = (
                        text: earlier.text, center: CGPoint(x: earlier.center.x, y: current.center.y - height - gap)
                    )
                    let pushed = (
                        text: current.text, center: CGPoint(x: current.center.x, y: earlier.center.y + height + gap)
                    )
                    if pushed.center.y + height / 2 <= size.height || size.height < 100 * scale {
                        result[index] = Disc(
                            marker: result[index].marker, center: result[index].center, callout: pushed)
                    } else {
                        result[earlierIndex] = Disc(
                            marker: result[earlierIndex].marker, center: result[earlierIndex].center, callout: lifted)
                    }
                }
            }
            return result
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
            if size.height >= 100 * scale {
                if y - height / 2 < 0 { y = disc.y + radius + height / 2 + 2 }
                if y + height / 2 > size.height { y = disc.y - radius - height / 2 - 2 }
            }
            let half = (CGFloat(text.count) * 6.4 + 24) * scale / 2
            let anchor = disc.x > size.width / 2 ? disc.x + radius - half : disc.x - radius + half
            return CGPoint(x: min(max(anchor, half), size.width - half), y: y)
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
                    .foregroundStyle(Color.white)
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

    private var glyphColor: Color {
        switch kind {
        case .buy: MonacoTheme.profit
        case .sell: MonacoTheme.loss
        case .seal: Color.white
        }
    }

    var body: some View {
        HStack(spacing: 4 * scale) {
            Text(glyph)
                .foregroundStyle(glyphColor)
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
