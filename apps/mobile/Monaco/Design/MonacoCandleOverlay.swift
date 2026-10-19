import MonacoCore
import SwiftUI

enum MonacoChartPrice {
    static func digits(in domain: ClosedRange<Double>) -> Int {
        let magnitude = max(abs(domain.lowerBound), abs(domain.upperBound))
        let span = domain.upperBound - domain.lowerBound
        if magnitude < 1 || span < 0.05 { return 4 }
        return span < 0.5 ? 3 : 2
    }

    static func text(_ value: Double, in domain: ClosedRange<Double>) -> String {
        value.formatted(.number.precision(.fractionLength(digits(in: domain))))
    }
}

struct MonacoCandleOverlay: View {
    let store: MonacoBarStore
    let window: ChartWindow
    let domain: ClosedRange<Double>
    let plot: CGRect
    let chartWidth: CGFloat
    let style: MonacoChartStyle
    let tint: Color
    let lastPrice: Double?
    let baseline: Double?
    let tagTint: Color?
    let showsLastPriceRule: Bool
    let showsEndDot: Bool
    let selectionStyle: MonacoChartSelectionStyle
    let isLive: Bool
    let isAtLatest: Bool
    let selection: Int?
    let intervalSeconds: Int
    let valueText: ((Double) -> String)?
    let tooltipAmount: ((Int) -> String?)?
    let onJump: () -> Void

    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    private var bars: [PlottedBar] { store.bars }

    private var tagWidth: CGFloat { max(chartWidth - plot.maxX - 2, 44) }

    private func x(_ date: Date) -> CGFloat {
        guard window.visibleSeconds > 0 else { return plot.minX }
        let fraction = date.timeIntervalSince(window.scrollDate) / window.visibleSeconds
        return plot.minX + CGFloat(fraction) * plot.width
    }

    private func y(_ value: Double) -> CGFloat {
        let span = domain.upperBound - domain.lowerBound
        guard span > 0 else { return plot.midY }
        return plot.minY + CGFloat((domain.upperBound - value) / span) * plot.height
    }

    var body: some View {
        ZStack(alignment: .topLeading) {
            axisLabels
            baselineMark
            lastPriceMarks
            lineDot
            crosshair
            jumpButton
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
    }

    @ViewBuilder
    private var axisLabels: some View {
        if style == .candles { timeLabels }
    }

    private var timeLabels: some View {
        let step = ChartAxisTicks.step(visibleSeconds: window.visibleSeconds)
        let candidates = ChartAxisTicks.dates(
            from: window.scrollDate, to: window.endDate, step: step, limit: 24,
            anchor: ChartAxisTicks.anchor(forBarSeconds: intervalSeconds, step: step))
        let dates = ChartAxisTicks.fitting(
            candidates,
            scrollDate: window.scrollDate,
            visibleSeconds: window.visibleSeconds,
            plotWidth: Double(plot.width),
            labelWidth: { Double(ChartAxisTicks.text($0, step: step).count) * 8 + 6 }
        )
        return ForEach(dates, id: \.self) { date in
            Text(ChartAxisTicks.text(date, step: step))
                .font(MonacoTheme.Typo.stamp)
                .foregroundStyle(MonacoTheme.tertiaryText)
                .fixedSize()
                .position(x: x(date), y: plot.maxY + MonacoCandleChart.axisHeight / 2)
                .allowsHitTesting(false)
                .accessibilityHidden(true)
        }
    }

    @ViewBuilder
    private var baselineMark: some View {
        if let baseline, domain.contains(baseline) {
            Path { path in
                path.move(to: CGPoint(x: plot.minX, y: y(baseline)))
                path.addLine(to: CGPoint(x: plot.maxX, y: y(baseline)))
            }
            .stroke(MonacoTheme.secondaryText, style: StrokeStyle(lineWidth: 1, dash: [3, 4]))
            .allowsHitTesting(false)
            .accessibilityHidden(true)
        }
    }

    @ViewBuilder
    private var lastPriceMarks: some View {
        if let lastPrice, selection == nil {
            let clamped = min(max(y(lastPrice), plot.minY + 10), plot.maxY - 10)
            if showsLastPriceRule, domain.contains(lastPrice) {
                Path { path in
                    path.move(to: CGPoint(x: plot.minX, y: clamped))
                    path.addLine(to: CGPoint(x: plot.maxX, y: clamped))
                }
                .stroke(MonacoTheme.secondaryText, style: StrokeStyle(lineWidth: 1, dash: [3, 4]))
                .allowsHitTesting(false)
            }
            priceTag(
                lastPrice, at: clamped, fill: tagTint ?? (style == .line ? tint : MonacoTheme.profitVivid),
                label: MonacoTheme.onVivid)
        }
    }

    private func priceTag(_ value: Double, at yPosition: CGFloat, fill: Color, label: Color) -> some View {
        Text(valueText?(value) ?? MonacoChartPrice.text(value, in: domain))
            .font(MonacoTheme.Typo.stamp)
            .foregroundStyle(label)
            .lineLimit(1)
            .minimumScaleFactor(0.7)
            .padding(.horizontal, MonacoTheme.Space.xs)
            .frame(width: tagWidth, height: 20)
            .background(Capsule().fill(fill))
            .position(x: plot.maxX + tagWidth / 2 + 2, y: yPosition)
            .allowsHitTesting(false)
            .accessibilityHidden(true)
    }

    private var endDotDiameter: CGFloat {
        showsEndDot ? MonacoLineStyle.endDotRadius * 2 : 8
    }

    @ViewBuilder
    private var lineDot: some View {
        if style == .line, selection == nil, let last = bars.last, bars.count > 1 {
            let position = CGPoint(x: x(last.date), y: y(last.close))
            if plot.insetBy(dx: -1, dy: -1).contains(position) {
                ZStack {
                    if isLive {
                        Circle().fill(tint.opacity(0.22)).frame(width: 18, height: 18)
                        if !reduceMotion {
                            Circle()
                                .fill(tint.opacity(0.5))
                                .frame(width: 8, height: 8)
                                .phaseAnimator([false, true]) { halo, expanded in
                                    halo.scaleEffect(expanded ? 2.4 : 1).opacity(expanded ? 0 : 1)
                                } animation: { expanded in
                                    expanded ? .easeOut(duration: 1.5) : nil
                                }
                        }
                    }
                    Circle().fill(tint).frame(width: endDotDiameter, height: endDotDiameter)
                }
                .position(position)
                .allowsHitTesting(false)
            }
        }
    }

    @ViewBuilder
    private var crosshair: some View {
        if let selection, bars.indices.contains(selection) {
            let bar = bars[selection]
            let position = CGPoint(x: x(bar.date), y: min(max(y(bar.close), plot.minY), plot.maxY))
            ZStack(alignment: .topLeading) {
                switch (style, selectionStyle) {
                case (.line, .tooltipPill): tooltipCrosshair(bar, index: selection, at: position)
                case (.line, .axisTag): lineCrosshair(bar, at: position)
                case (.candles, _): candleCrosshair(bar, at: position)
                }
            }
            .allowsHitTesting(false)
            .accessibilityIdentifier("asset-chart-crosshair")
        }
    }

    private func candleCrosshair(_ bar: PlottedBar, at position: CGPoint) -> some View {
        ZStack(alignment: .topLeading) {
            Path { path in
                path.move(to: CGPoint(x: position.x, y: plot.minY))
                path.addLine(to: CGPoint(x: position.x, y: plot.maxY))
                path.move(to: CGPoint(x: plot.minX, y: position.y))
                path.addLine(to: CGPoint(x: plot.maxX, y: position.y))
            }
            .stroke(MonacoTheme.secondaryText, style: StrokeStyle(lineWidth: 1, dash: [3, 3]))
            Circle()
                .fill(MonacoTheme.ink)
                .frame(width: 8, height: 8)
                .position(position)
            priceTag(bar.close, at: position.y, fill: MonacoTheme.ink, label: MonacoTheme.canvas)
            timePill(bar.date)
        }
    }

    private func lineCrosshair(_ bar: PlottedBar, at position: CGPoint) -> some View {
        ZStack(alignment: .topLeading) {
            Path { path in
                path.move(to: CGPoint(x: position.x, y: plot.minY))
                path.addLine(to: CGPoint(x: position.x, y: plot.maxY))
            }
            .stroke(MonacoTheme.secondaryText.opacity(0.7), style: StrokeStyle(lineWidth: 1))
            Circle()
                .fill(tint)
                .frame(width: 10, height: 10)
                .overlay(Circle().stroke(MonacoTheme.canvas, lineWidth: 2))
                .position(position)
            timePill(bar.date)
        }
    }

    private func tooltipCrosshair(_ bar: PlottedBar, index: Int, at position: CGPoint) -> some View {
        let date = ChartCrosshairLabel.text(
            bar.date, visibleSeconds: window.visibleSeconds, intervalSeconds: intervalSeconds)
        return ChartTooltip(
            position: position, plot: plot, tint: tint,
            text: [date, tooltipAmount?(index)].compactMap { $0 }.joined(separator: " · "))
    }

    private func timePill(_ date: Date) -> some View {
        let text = ChartCrosshairLabel.text(
            date, visibleSeconds: window.visibleSeconds, intervalSeconds: intervalSeconds)
        let width = CGFloat(text.count) * 8 + 16
        let center = min(max(x(date), plot.minX + width / 2), plot.maxX - width / 2)
        return Text(text)
            .font(MonacoTheme.Typo.stamp)
            .foregroundStyle(MonacoTheme.canvas)
            .lineLimit(1)
            .fixedSize()
            .padding(.horizontal, MonacoTheme.Space.s)
            .frame(height: 22)
            .background(Capsule().fill(MonacoTheme.ink))
            .position(x: center, y: plot.minY + 11)
    }

    @ViewBuilder
    private var jumpButton: some View {
        if !isAtLatest && selection == nil {
            Button(action: onJump) {
                Image(systemName: "chevron.right.2")
                    .font(MonacoTheme.Typo.subheadStrong)
                    .frame(width: 20, height: 20)
            }
            .buttonStyle(.monacoCompact)
            .accessibilityLabel("Jump to latest")
            .accessibilityIdentifier("asset-chart-jump")
            .position(x: plot.maxX - 30, y: plot.maxY - 30)
            .transition(.opacity)
        }
    }
}

private struct ChartTooltip: View {
    let position: CGPoint
    let plot: CGRect
    let tint: Color
    let text: String

    var body: some View {
        ZStack(alignment: .topLeading) {
            Path { path in
                path.move(to: CGPoint(x: position.x, y: plot.minY))
                path.addLine(to: CGPoint(x: position.x, y: plot.maxY))
            }
            .stroke(MonacoTheme.border, style: StrokeStyle(lineWidth: ChartMetrics.crosshairWidth))
            Circle()
                .fill(tint)
                .frame(width: MonacoLineStyle.endDotRadius * 2, height: MonacoLineStyle.endDotRadius * 2)
                .position(position)
            TooltipPillLayout(centerX: position.x, top: plot.minY) {
                Text(text)
                    .font(MonacoTheme.Typo.captionStrong)
                    .foregroundStyle(MonacoTheme.background)
                    .multilineTextAlignment(.center)
                    .fixedSize(horizontal: false, vertical: true)
                    .padding(.vertical, MonacoTheme.Space.xs)
                    .padding(.horizontal, MonacoTheme.Space.s)
                    .background(
                        RoundedRectangle(cornerRadius: ChartMetrics.tooltipRadius, style: .continuous)
                            .fill(MonacoTheme.primaryText))
            }
        }
    }
}

private struct TooltipPillLayout: Layout {
    let centerX: CGFloat
    let top: CGFloat

    func sizeThatFits(proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) -> CGSize {
        proposal.replacingUnspecifiedDimensions(by: .zero)
    }

    func placeSubviews(in bounds: CGRect, proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) {
        guard let pill = subviews.first else { return }
        let size = pill.sizeThatFits(ProposedViewSize(width: bounds.width, height: nil))
        let x = min(max(bounds.minX + centerX, bounds.minX + size.width / 2), bounds.maxX - size.width / 2)
        pill.place(at: CGPoint(x: x, y: bounds.minY + top), anchor: .top, proposal: ProposedViewSize(size))
    }
}
