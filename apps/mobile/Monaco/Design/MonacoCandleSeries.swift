import Charts
import MonacoCore
import SwiftUI

struct MonacoCandleSeries: View, Equatable {
    let intervalSeconds: Int
    let style: MonacoChartStyle
    let domain: ClosedRange<Double>
    let xDomain: ClosedRange<Date>
    let visibleSeconds: TimeInterval
    let scrollRevision: Int
    let scrollLocked: Bool
    let scrollPosition: Binding<Date>
    let plot: Binding<CGRect>

    static func == (lhs: Self, rhs: Self) -> Bool {
        lhs.intervalSeconds == rhs.intervalSeconds && lhs.style == rhs.style && lhs.domain == rhs.domain
            && lhs.xDomain == rhs.xDomain && lhs.visibleSeconds == rhs.visibleSeconds
            && lhs.scrollRevision == rhs.scrollRevision && lhs.scrollLocked == rhs.scrollLocked
    }

    var body: some View {
        Chart {
            PointMark(x: .value("Time", xDomain.upperBound), y: .value("Price", domain.lowerBound))
                .opacity(0)
        }
        .chartXScale(domain: xDomain)
        .chartYScale(domain: domain)
        .chartScrollableAxes(.horizontal)
        .chartXVisibleDomain(length: visibleSeconds)
        .chartScrollPosition(x: scrollPosition)
        .scrollDisabled(scrollLocked)
        .chartXAxis(.hidden)
        .chartYAxis { if style == .candles { yAxis } }
        .chartPlotStyle { $0.background(Color.clear) }
        .chartOverlay { proxy in
            GeometryReader { geometry in
                Color.clear
                    .onChange(of: frame(proxy, geometry), initial: true) { _, new in
                        if plot.wrappedValue != new { plot.wrappedValue = new }
                    }
                    .allowsHitTesting(false)
            }
        }
    }

    private func frame(_ proxy: ChartProxy, _ geometry: GeometryProxy) -> CGRect {
        guard let anchor = proxy.plotFrame else { return .zero }
        let content = geometry[anchor]
        let span = xDomain.upperBound.timeIntervalSince(xDomain.lowerBound)
        guard span > 0 else { return .zero }
        let fraction = min(max(visibleSeconds / span, 0), 1)
        return CGRect(x: 0, y: content.minY, width: content.width * fraction, height: content.height)
    }

    private var yAxis: some AxisContent {
        AxisMarks(position: .trailing, values: .automatic(desiredCount: 4)) { value in
            AxisGridLine(stroke: StrokeStyle(lineWidth: 0.5))
                .foregroundStyle(MonacoTheme.hairline)
            AxisValueLabel {
                if let price = value.as(Double.self) {
                    Text(MonacoChartPrice.text(price, in: domain))
                        .font(MonacoTheme.Typo.stamp)
                        .foregroundStyle(MonacoTheme.tertiaryText)
                }
            }
        }
    }
}
