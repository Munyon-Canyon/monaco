import MonacoCore
import SwiftUI

enum AssetDetailMetrics {
    static let chartHeight: CGFloat = 170
}

struct AssetDetailChartSection: View {
    let detail: AssetDetailPresentation
    let model: AssetDetailClientModel
    @Binding var scrubbedIndex: Int?
    @Binding var style: MonacoChartStyle

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            controls
            content
            MonacoRangeChips(
                ranges: AssetChartRange.allCases, selection: model.selectedRange, identifierPrefix: "asset-range",
                label: { $0.label },
                onSelect: { range in Task { await model.loadChart(range: range) } }
            )
            .accessibilityIdentifier("asset-chart-ranges")
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
    }

    @ViewBuilder private var controls: some View {
        if model.chartPhase == .loaded, model.chart?.isDrawable == true {
            ViewThatFits(in: .horizontal) {
                HStack(spacing: MonacoTheme.Space.s) {
                    intervalSelect
                    Spacer(minLength: 0)
                    MonacoChartStyleToggle(style: $style)
                }
                VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                    intervalSelect
                    MonacoChartStyleToggle(style: $style)
                }
            }
        }
    }

    private var intervalSelect: some View {
        MonacoMenuSelect(
            title: "Bar interval",
            selection: Binding(get: { model.selectedInterval }, set: { selectInterval($0) }),
            options: ChartInterval.allowed(for: model.selectedRange), label: { $0.label }
        )
        .accessibilityIdentifier("asset-chart-interval")
    }

    private func selectInterval(_ interval: ChartInterval) {
        guard interval != model.selectedInterval else { return }
        Haptics.selection()
        scrubbedIndex = nil
        Task { await model.loadChart(range: model.selectedRange, interval: interval) }
    }

    @ViewBuilder private var content: some View {
        switch model.chartPhase {
        case .idle, .loading:
            SkeletonBlock(height: AssetDetailMetrics.chartHeight, radius: MonacoTheme.Radius.card)
                .accessibilityIdentifier("asset-detail-chart-loading")
        case .failed:
            MonacoErrorRow(thing: "price history", identifier: "asset-detail-chart-failed", inset: false) {
                Task { await model.loadChart(range: model.selectedRange) }
            }
        case .loaded:
            if let chart = model.chart, chart.points.count >= 2 {
                candleChart(chart)
            } else {
                QuietChart(message: Self.emptyMessage(for: detail.kind))
                    .accessibilityIdentifier("asset-detail-chart-empty")
            }
        }
    }

    private static func emptyMessage(for kind: AssetKind) -> String {
        let how = kind == .preIpo ? PreIpoCopy.chartEmptyMessage : "The curve draws as it trades."
        return "\(PreIpoCopy.chartEmpty) \(how)"
    }

    private func candleChart(_ chart: AssetChartSeries) -> some View {
        MonacoCandleChart(
            bars: drawnBars(chart),
            intervalSeconds: chart.intervalSeconds,
            style: style,
            tint: tone,
            lastPrice: chart.lastPoint?.chartValue,
            isLive: detail.session == .open && chart.range == .oneDay,
            showsEndDot: true,
            selectionStyle: .tooltipPill,
            resetKey: "\(chart.range.rawValue)-\(chart.interval.rawValue)",
            hasOlder: chart.hasOlder && chart.bars.count < AssetDetailClientModel.maximumBars,
            retryToken: model.olderFailureTick,
            tooltipAmount: { chart.barPoint(at: $0).map { UsdAmountFormatter.format(micros: $0.priceUsdcMicros) } },
            height: AssetDetailMetrics.chartHeight,
            selection: $scrubbedIndex,
            onNeedOlder: { Task { await model.loadOlder() } },
            summary: "\(chart.range.label) price history, \(chart.interval.label) bars",
            nearestIndex: { chart.nearestBarIndex(to: $0) },
            describeBar: { chart.describeBar(at: $0) ?? "" },
            accessibilityIdentifier: "asset-detail-chart"
        )
    }

    private func drawnBars(_ chart: AssetChartSeries) -> [PlottedBar] {
        #if DEBUG
        if ProcessInfo.processInfo.arguments.contains("-MonacoRawCandles") { return chart.bars }
        #endif
        return chart.candleBars
    }

    private var tone: Color {
        (model.rangeChange?.basisPoints ?? 0) >= 0 ? MonacoTheme.profitVivid : MonacoTheme.lossVivid
    }
}
