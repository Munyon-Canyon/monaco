import MonacoCore
import SwiftUI

nonisolated enum PreIpoChartMetrics {
    static let plotHeight: CGFloat = 112
    static let dotRadius: CGFloat = 3.5
    static let hollowStroke: CGFloat = 1.7
    static let dash: [CGFloat] = [4, 4]
    static let minimumSlots = 4
    static let slotPadding: CGFloat = 0.1
    static let floorDivisor: Int64 = 5
    static let shareScale: Int64 = 1_000
    static let topInset: CGFloat = 6
}

nonisolated struct PreIpoChartGeometry: Equatable {
    let xs: [CGFloat]
    let ys: [CGFloat]
    let baseline: CGFloat
    let width: CGFloat

    init(steps: [PreIpoStep], width: CGFloat, height: CGFloat) {
        self.width = width
        let slots = CGFloat(max(steps.count, PreIpoChartMetrics.minimumSlots)) + PreIpoChartMetrics.slotPadding
        let spacing = width / slots
        let edge = PreIpoChartMetrics.dotRadius
        xs = steps.indices.map { edge + CGFloat($0) * spacing }
        let prices = steps.map(\.priceMicros)
        let high = prices.max() ?? 0
        let low = prices.min() ?? 0
        let floor = low - (high - low) / PreIpoChartMetrics.floorDivisor
        let top = PreIpoChartMetrics.topInset
        let usable = max(height - top, 1)
        ys = prices.map { price in
            guard high > floor else { return top }
            let share = (price - floor) * PreIpoChartMetrics.shareScale / (high - floor)
            return top + usable * (1 - CGFloat(share) / CGFloat(PreIpoChartMetrics.shareScale))
        }
        baseline = height
    }

    var estimateIndex: Int? { xs.indices.last }
}

struct PreIpoRoundsChart: View {
    let steps: [PreIpoStep]

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            plot
            labels
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("asset-pre-ipo-chart")
    }

    private var plot: some View {
        GeometryReader { proxy in
            let geometry = PreIpoChartGeometry(
                steps: steps, width: proxy.size.width, height: proxy.size.height)
            ZStack {
                PreIpoStepFill(geometry: geometry, roundCount: roundCount)
                    .fill(MonacoTheme.profitVivid.opacity(MonacoLineStyle.fillOpacity))
                PreIpoStepLine(geometry: geometry, roundCount: roundCount)
                    .stroke(MonacoTheme.profitVivid, style: MonacoLineStyle.stroke)
                PreIpoEstimateStep(geometry: geometry, roundCount: roundCount)
                    .stroke(
                        MonacoTheme.secondaryText,
                        style: StrokeStyle(
                            lineWidth: MonacoLineStyle.stroke.lineWidth, lineCap: .round,
                            dash: PreIpoChartMetrics.dash))
                dots(geometry)
            }
        }
        .frame(height: PreIpoChartMetrics.plotHeight)
        .accessibilityHidden(true)
    }

    private var roundCount: Int { steps.filter { !$0.isEstimate }.count }

    private func dots(_ geometry: PreIpoChartGeometry) -> some View {
        ZStack {
            ForEach(Array(steps.enumerated()), id: \.element.id) { index, step in
                let radius = PreIpoChartMetrics.dotRadius
                Group {
                    if step.isEstimate {
                        Circle()
                            .fill(MonacoTheme.canvas)
                            .overlay(
                                Circle().strokeBorder(
                                    MonacoTheme.secondaryText, lineWidth: PreIpoChartMetrics.hollowStroke))
                    } else {
                        Circle().fill(MonacoTheme.profitVivid)
                    }
                }
                .frame(width: radius * 2, height: radius * 2)
                .position(x: geometry.xs[index], y: geometry.ys[index])
            }
        }
    }

    private var labels: some View {
        let stacked = dynamicTypeSize.isAccessibilitySize
        return Group {
            if stacked {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                    ForEach(steps) { label($0) }
                }
                .padding(.horizontal, MonacoTheme.Space.gutter)
            } else {
                GeometryReader { proxy in
                    let geometry = PreIpoChartGeometry(
                        steps: steps, width: proxy.size.width, height: PreIpoChartMetrics.plotHeight)
                    ZStack(alignment: .topLeading) {
                        ForEach(Array(steps.enumerated()), id: \.element.id) { index, step in
                            label(step)
                                .fixedSize()
                                .offset(x: max(geometry.xs[index], MonacoTheme.Space.gutter))
                        }
                    }
                }
                .frame(height: labelHeight)
            }
        }
    }

    @ScaledMetric(relativeTo: .subheadline) private var labelHeight: CGFloat = 48

    private func label(_ step: PreIpoStep) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            Text(step.dateLabel)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
            Text(step.priceLabel)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.primaryText)
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(step.spokenLabel)
    }
}

private nonisolated struct PreIpoStepLine: Shape {
    let geometry: PreIpoChartGeometry
    let roundCount: Int

    func path(in rect: CGRect) -> Path {
        var path = Path()
        guard roundCount >= 1 else { return path }
        let last = roundCount - 1
        path.move(to: CGPoint(x: geometry.xs[0], y: geometry.ys[0]))
        if last >= 1 {
            for index in 1...last {
                path.addLine(to: CGPoint(x: geometry.xs[index], y: geometry.ys[index - 1]))
                path.addLine(to: CGPoint(x: geometry.xs[index], y: geometry.ys[index]))
            }
        }
        if let estimate = geometry.estimateIndex, estimate > last {
            path.addLine(to: CGPoint(x: geometry.xs[estimate], y: geometry.ys[last]))
        }
        return path
    }
}

private nonisolated struct PreIpoStepFill: Shape {
    let geometry: PreIpoChartGeometry
    let roundCount: Int

    func path(in rect: CGRect) -> Path {
        var path = PreIpoStepLine(geometry: geometry, roundCount: roundCount).path(in: rect)
        guard roundCount >= 1, let end = path.currentPoint else { return path }
        path.addLine(to: CGPoint(x: end.x, y: geometry.baseline))
        path.addLine(to: CGPoint(x: geometry.xs[0], y: geometry.baseline))
        path.closeSubpath()
        return path
    }
}

private nonisolated struct PreIpoEstimateStep: Shape {
    let geometry: PreIpoChartGeometry
    let roundCount: Int

    func path(in rect: CGRect) -> Path {
        var path = Path()
        guard let estimate = geometry.estimateIndex, estimate >= roundCount else { return path }
        let x = geometry.xs[estimate]
        let y = geometry.ys[estimate]
        if roundCount >= 1 {
            path.move(to: CGPoint(x: x, y: geometry.ys[roundCount - 1]))
            path.addLine(to: CGPoint(x: x, y: y))
        } else {
            path.move(to: CGPoint(x: x, y: y))
        }
        path.addLine(to: CGPoint(x: geometry.width, y: y))
        return path
    }
}
