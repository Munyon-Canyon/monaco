import MonacoCore
import SwiftUI

final class MonacoBarBox: Sendable {
    let bars: [PlottedBar]

    init(_ bars: [PlottedBar]) {
        self.bars = bars
    }
}

struct MonacoBarStore: Equatable {
    let box: MonacoBarBox

    init(_ bars: [PlottedBar]) {
        box = MonacoBarBox(bars)
    }

    var bars: [PlottedBar] { box.bars }

    static func == (lhs: Self, rhs: Self) -> Bool { lhs.box === rhs.box }
}

struct MonacoCandleCanvas: View {
    let store: MonacoBarStore
    let window: ChartWindow
    let domain: ClosedRange<Double>
    let plot: CGRect
    let intervalSeconds: Int
    let style: MonacoChartStyle
    let tint: Color
    let selection: Int?

    var body: some View {
        Canvas { context, _ in
            let bars = store.bars
            guard plot.width > 0, plot.height > 0, window.visibleSeconds > 0 else { return }
            let range = window.visibleRange(of: bars, slack: TimeInterval(intervalSeconds))
            guard !range.isEmpty else { return }
            context.clip(to: Path(plot))
            let metrics = Metrics(window: window, domain: domain, plot: plot, intervalSeconds: intervalSeconds)
            switch style {
            case .candles: drawCandles(&context, bars[range], metrics)
            case .line: drawLine(&context, bars[range], metrics, held: heldX(bars, metrics))
            }
        }
        .allowsHitTesting(false)
    }

    private struct Metrics {
        let window: ChartWindow
        let domain: ClosedRange<Double>
        let plot: CGRect
        let intervalSeconds: Int

        func x(_ date: Date) -> CGFloat {
            let fraction = date.timeIntervalSince(window.scrollDate) / window.visibleSeconds
            return plot.minX + CGFloat(fraction) * plot.width
        }

        func y(_ value: Double) -> CGFloat {
            let span = domain.upperBound - domain.lowerBound
            guard span > 0 else { return plot.midY }
            return plot.minY + CGFloat((domain.upperBound - value) / span) * plot.height
        }

        var bodyWidth: CGFloat {
            max(plot.width * CGFloat(intervalSeconds) / CGFloat(window.visibleSeconds) * 0.7, 1)
        }
    }

    private func drawCandles(_ context: inout GraphicsContext, _ visible: ArraySlice<PlottedBar>, _ metrics: Metrics) {
        let width = metrics.bodyWidth
        var rising = Path()
        var falling = Path()
        for bar in visible {
            let x = metrics.x(bar.date)
            let top = metrics.y(max(bar.open, bar.close))
            let bottom = metrics.y(min(bar.open, bar.close))
            let bodyHeight = max(bottom - top, Self.minimumBodyHeight)
            let bodyTop = (top + bottom) / 2 - bodyHeight / 2
            var shape = Path()
            shape.addRect(
                CGRect(x: x - 0.5, y: metrics.y(bar.high), width: 1, height: metrics.y(bar.low) - metrics.y(bar.high)))
            shape.addRect(CGRect(x: x - width / 2, y: bodyTop, width: width, height: bodyHeight))
            if bar.isRising {
                rising.addPath(shape)
            } else {
                falling.addPath(shape)
            }
        }
        context.fill(rising, with: .color(MonacoTheme.profitVivid))
        context.fill(falling, with: .color(MonacoTheme.lossVivid))
    }

    private func heldX(_ bars: [PlottedBar], _ metrics: Metrics) -> CGFloat? {
        guard let selection, bars.indices.contains(selection) else { return nil }
        return metrics.x(bars[selection].date)
    }

    private func drawLine(
        _ context: inout GraphicsContext, _ visible: ArraySlice<PlottedBar>, _ metrics: Metrics, held: CGFloat?
    ) {
        let xs = visible.map { Double(metrics.x($0.date)) }
        let ys = visible.map { Double(metrics.y($0.close)) }
        guard let line = MonacoLineStyle.line(xs: xs, ys: ys) else { return }
        guard let held else {
            context.stroke(line, with: .color(tint), style: MonacoLineStyle.stroke)
            return
        }
        let split = min(max(held, metrics.plot.minX), metrics.plot.maxX)
        let plot = metrics.plot
        context.drawLayer { layer in
            layer.clip(to: Path(CGRect(x: plot.minX, y: plot.minY, width: split - plot.minX, height: plot.height)))
            layer.stroke(line, with: .color(tint), style: MonacoLineStyle.stroke)
        }
        context.drawLayer { layer in
            layer.opacity = 0.3
            layer.clip(to: Path(CGRect(x: split, y: plot.minY, width: plot.maxX - split, height: plot.height)))
            layer.stroke(line, with: .color(tint), style: MonacoLineStyle.stroke)
        }
    }

    private static let minimumBodyHeight: CGFloat = 1.5
}
