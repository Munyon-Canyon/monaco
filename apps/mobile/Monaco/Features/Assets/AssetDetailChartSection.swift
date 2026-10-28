import MonacoCore
import SwiftUI

enum AssetDetailMetrics {
    static let chartHeight: CGFloat = 170
}

struct AssetDetailChartSection: View {
    let detail: AssetDetailPresentation
    let model: AssetDetailClientModel
    @Binding var scrubbedIndex: Int?

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            content
            MonacoRangeChips(
                ranges: AssetChartRange.stockPage, selection: model.selectedRange, identifierPrefix: "asset-range",
                label: { $0.label },
                onSelect: { range in
                    scrubbedIndex = nil
                    Task { await model.loadChart(range: range) }
                }
            )
            .accessibilityIdentifier("asset-chart-ranges")
        }
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
                lineChart(chart)
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

    private func lineChart(_ chart: AssetChartSeries) -> some View {
        MonacoCandleChart(
            bars: chart.candleBars,
            intervalSeconds: chart.intervalSeconds,
            style: .line,
            tint: tone,
            lastPrice: chart.lastPoint?.chartValue,
            isLive: detail.session == .open && (chart.range == .oneDay || chart.range == .oneHour),
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

    private var tone: Color {
        (model.rangeChange?.basisPoints ?? 0) >= 0 ? MonacoTheme.profitVivid : MonacoTheme.lossVivid
    }
}
