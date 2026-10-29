import MonacoCore
import SwiftUI

enum FeedSaleChartMetrics {
    static let height: CGFloat = 64
    static let disc: CGFloat = 20
    static let ring: CGFloat = 2
}

struct FeedSaleChartGeometry {
    let size: CGSize
    private let times: ClosedRange<Double>
    private let prices: ClosedRange<Int64>
    private let inset: CGFloat

    init(
        size: CGSize, disc: CGFloat = FeedSaleChartMetrics.disc, points: [FeedPostFacts.ChartPoint],
        markers: [FeedPostFacts.ChartMarker]
    ) {
        self.size = size
        inset = disc / 2 + FeedSaleChartMetrics.ring
        let at = points.map { $0.at.timeIntervalSinceReferenceDate }
        let price = points.map(\.priceMicros) + markers.map(\.priceMicros)
        times = (at.min() ?? 0)...(at.max() ?? 0)
        prices = (price.min() ?? 0)...(price.max() ?? 0)
    }

    func x(_ date: Date) -> Double {
        let span = times.upperBound - times.lowerBound
        let fraction = span > 0 ? (date.timeIntervalSinceReferenceDate - times.lowerBound) / span : 0.5
        return inset + min(max(fraction, 0), 1) * max(size.width - inset * 2, 0)
    }

    func y(_ priceMicros: Int64) -> Double {
        let span = prices.upperBound - prices.lowerBound
        let fraction = span > 0 ? CGFloat(priceMicros - prices.lowerBound) / CGFloat(span) : 0.5
        return size.height - inset - min(max(fraction, 0), 1) * max(size.height - inset * 2, 0)
    }
}

struct FeedSaleChart: View {
    let points: [FeedPostFacts.ChartPoint]
    let markers: [FeedPostFacts.ChartMarker]
    let isLoss: Bool

    @ScaledMetric(relativeTo: .caption2) private var disc = FeedSaleChartMetrics.disc
    @ScaledMetric(relativeTo: .caption2) private var height = FeedSaleChartMetrics.height

    private var lineColor: Color { isLoss ? MonacoTheme.lossVivid : MonacoTheme.profitVivid }

    var body: some View {
        Canvas { context, size in
            let geometry = FeedSaleChartGeometry(size: size, disc: disc, points: points, markers: markers)
            let xs = points.map { geometry.x($0.at) }
            let ys = points.map { geometry.y($0.priceMicros) }
            if let area = MonacoLineStyle.area(xs: xs, ys: ys, baseline: size.height) {
                context.fill(area, with: .color(lineColor.opacity(MonacoLineStyle.fillOpacity)))
            }
            if let line = MonacoLineStyle.line(xs: xs, ys: ys) {
                context.stroke(line, with: .color(lineColor), style: MonacoLineStyle.stroke)
            }
            for marker in markers {
                draw(marker, at: CGPoint(x: geometry.x(marker.at), y: geometry.y(marker.priceMicros)), in: &context)
            }
        }
        .frame(maxWidth: .infinity)
        .frame(height: height)
        .accessibilityHidden(true)
    }

    private func draw(_ marker: FeedPostFacts.ChartMarker, at center: CGPoint, in context: inout GraphicsContext) {
        let isBuy = marker.side == .buy
        let radius = disc / 2
        let ring = radius + FeedSaleChartMetrics.ring
        context.fill(circle(at: center, radius: ring), with: .color(MonacoTheme.canvas))
        context.fill(
            circle(at: center, radius: radius), with: .color(isBuy ? MonacoTheme.profitVivid : MonacoTheme.lossVivid))
        let letter = Text(isBuy ? "B" : "S")
            .font(Font.caption2.weight(.heavy))
            .foregroundStyle(isBuy ? MonacoTheme.canvas : Color.white)
        context.draw(letter, at: center)
    }

    private func circle(at center: CGPoint, radius: CGFloat) -> Path {
        Path(ellipseIn: CGRect(x: center.x - radius, y: center.y - radius, width: radius * 2, height: radius * 2))
    }
}
