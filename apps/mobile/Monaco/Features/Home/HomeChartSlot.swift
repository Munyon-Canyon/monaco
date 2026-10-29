import MonacoAPI
import MonacoCore
import SwiftUI

enum HomeChartSlot: HomeSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        HomeChartView()
    }
}

struct HomeChartView: View {
    @Environment(PortfolioModel.self) private var portfolio: PortfolioModel?
    @Environment(\.homeReads) private var reads
    @State private var selection: Int?

    var body: some View {
        Group {
            if reads?.plan.showsSharedRow == true || portfolio?.summary == nil
                || portfolio?.summary?.isFirstUse == true
            {
                EmptyView()
            } else if let chart = portfolio?.chart {
                content(chart)
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("home-chart")
    }

    @ViewBuilder private func content(_ chart: ValueChartModel) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            switch chart.state {
            case .idle, .loading:
                chartSkeleton
                MonacoRangeChipsSkeleton(ranges: LeaderboardRange.figurePeriods)
                    .padding(.horizontal, MonacoTheme.Space.gutter)
            case .failed:
                MonacoErrorRow(thing: "the chart", identifier: "home-portfolio-chart-failed") {
                    Task { await chart.load() }
                }
            case .loaded:
                let shown = chart.shownRange ?? chart.range
                plot(chart.curve.flatMap { $0.hasEnoughHistory ? $0 : nil }, range: shown)
                periods(chart)
                    .padding(.horizontal, MonacoTheme.Space.gutter)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private var chartSkeleton: some View {
        CurveScrubChartFrame(showsDateRange: false) { SkeletonBlock(height: nil, radius: MonacoTheme.Radius.cardTight) }
            .accessibilityElement(children: .ignore)
            .accessibilityLabel("Loading the chart")
            .accessibilityIdentifier("home-portfolio-chart-loading")
    }

    @ViewBuilder private func plot(_ curve: ValueCurve?, range: LeaderboardRange) -> some View {
        if let curve {
            CurveScrubChart(
                curve: curve, plot: .pnl, range: range, showsDateRange: false, selection: $selection,
                identifier: "home-pnl-chart")
        } else {
            CurveScrubChartFrame(showsDateRange: false) {
                Text(range.shortHistoryLine)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                    .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
                    .accessibilityIdentifier("home-portfolio-short")
            }
        }
    }

    private func periods(_ chart: ValueChartModel) -> some View {
        MonacoRangeChips(
            ranges: chart.ranges, selection: chart.range, identifierPrefix: "home-portfolio"
        ) { range in
            selection = nil
            Task { await chart.select(range) }
        }
    }
}
