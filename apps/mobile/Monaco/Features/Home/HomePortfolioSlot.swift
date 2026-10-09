import MonacoAPI
import MonacoCore
import SwiftUI

enum HomePortfolioSlot: HomeSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        VStack(spacing: 0) {
            HomeSharedErrorRow()
            HomePortfolioHero()
        }
    }
}

enum HeroRhythm {
    static let titleToBalance = MonacoTheme.Space.xs
    static let balanceToChip = MonacoTheme.Space.s
    static let chipToChart = MonacoTheme.Space.m
    static let withinChart = MonacoTheme.Space.s
}

private struct HomePortfolioHero: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(ScreenRefresh.self) private var refresh: ScreenRefresh?
    @Environment(PortfolioModel.self) private var portfolio: PortfolioModel?
    @Environment(\.homeReads) private var reads
    @State private var chart: ValueChartModel?
    @State private var selection: Int?

    var body: some View {
        Group {
            switch portfolio?.state ?? .loading {
            case .idle, .loading:
                HomePortfolioSkeleton()
            case .failed, .loaded:
                content
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                    .padding(.top, MonacoTheme.Space.sm)
                    .padding(.bottom, MonacoTheme.Space.l)
                    .frame(maxWidth: .infinity, alignment: .leading)
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("home-portfolio")
        .onChange(of: HomeReadStatus(portfolio?.state ?? .loading), initial: true) { _, status in
            reads?.report(.portfolio, status)
        }
        .task {
            let chart = preparedChart()
            refresh?.register("home-portfolio-chart") { await chart.load() }
            await withTaskGroup(of: Void.self) { group in
                group.addTask { await chart.load() }
                group.addTask { await chart.observe() }
            }
        }
        .onScreenVisibilityChange { chart?.setVisible($0) }
        .onChange(of: chart?.toast) { _, message in
            guard let message else { return }
            toasts.current = MonacoToast(message: message)
            chart?.dismissToast()
        }
    }

    @ViewBuilder private var content: some View {
        switch portfolio?.state ?? .loading {
        case .idle, .loading:
            EmptyView()
        case .failed:
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                HomePortfolioTitle()
                if reads.showsOwnRow(.portfolio) {
                    MonacoErrorRow(thing: "your portfolio", identifier: "home-portfolio-failed", inset: false) {
                        Task { await portfolio?.load() }
                    }
                }
            }
        case .loaded(let summary):
            VStack(alignment: .leading, spacing: HeroRhythm.chipToChart) {
                VStack(alignment: .leading, spacing: HeroRhythm.balanceToChip) {
                    VStack(alignment: .leading, spacing: HeroRhythm.titleToBalance) {
                        HomePortfolioTitle()
                        MoneyText(micros: summary.totalMicros, style: .hero)
                            .accessibilityIdentifier("home-portfolio-total")
                    }
                    chip(summary)
                }
                if !summary.isEmpty {
                    curve
                }
            }
        }
    }

    @ViewBuilder private func chip(_ summary: PortfolioSummary) -> some View {
        if summary.isEmpty {
            Text("\(summary.chip) · all time")
                .moneyFont(.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
                .accessibilityIdentifier("home-portfolio-chip")
        } else {
            HStack(spacing: MonacoTheme.Space.s) {
                PnLBadge(dollarPnl: summary.pnl, percentReturn: summary.returnText)
                    .accessibilityIdentifier("home-portfolio-chip")
                Text("all time")
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
            }
        }
    }

    private var hairline: some View {
        Rectangle()
            .fill(MonacoTheme.hairline)
            .frame(height: 1)
            .accessibilityHidden(true)
            .accessibilityIdentifier("home-portfolio-flat")
    }

    @ViewBuilder private var curve: some View {
        if let chart {
            VStack(alignment: .leading, spacing: HeroRhythm.withinChart) {
                curveRows(chart)
            }
        }
    }

    @ViewBuilder private func curveRows(_ chart: ValueChartModel) -> some View {
        switch chart.state {
        case .idle, .loading:
            CurveReadoutLine(readout: nil)
            SkeletonBlock(height: 160, radius: 12)
                .accessibilityIdentifier("home-portfolio-chart-loading")
            rangeChips(chart)
        case .failed:
            MonacoErrorRow(thing: "the chart", identifier: "home-portfolio-chart-failed", inset: false) {
                Task { await chart.load() }
            }
        case .loaded:
            CurveReadoutLine(readout: chart.curve.flatMap { curve in selection.flatMap { curve.readout(at: $0) } })
            if let curve = chart.curve, curve.hasEnoughHistory {
                CurveScrubChart(
                    curve: curve, range: chart.shownRange ?? chart.range, selection: $selection,
                    identifier: "home-pnl-chart")
            } else {
                VStack(alignment: .leading, spacing: HeroRhythm.withinChart) {
                    hairline
                    Text(chart.range.shortHistoryLine)
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.muted)
                        .accessibilityIdentifier("home-portfolio-short")
                }
                .frame(height: 160, alignment: .center)
            }
            rangeChips(chart)
        }
    }

    private func rangeChips(_ chart: ValueChartModel) -> some View {
        MonacoRangeChips(
            ranges: chart.ranges, selection: chart.range, identifierPrefix: "home-portfolio"
        ) { range in
            selection = nil
            Task { await chart.select(range) }
        }
    }

    private func preparedChart() -> ValueChartModel {
        if let chart { return chart }
        let chart = ValueChartModel(
            subjects: [.me], ranges: LeaderboardRange.allCases, range: .oneDay, api: environment.api,
            hints: environment.hints)
        self.chart = chart
        return chart
    }
}

private struct HomePortfolioTitle: View {
    var body: some View {
        Text("Your money in cabals")
            .font(MonacoTheme.Typo.callout)
            .foregroundStyle(MonacoTheme.muted)
            .accessibilityAddTraits(.isHeader)
    }
}

struct HomePortfolioSkeleton: View {
    var body: some View {
        VStack(alignment: .leading, spacing: HeroRhythm.chipToChart) {
            VStack(alignment: .leading, spacing: HeroRhythm.balanceToChip) {
                VStack(alignment: .leading, spacing: HeroRhythm.titleToBalance) {
                    HomePortfolioTitle().skeletonBar(width: 140)
                    MoneyText(micros: 0, style: .hero).skeletonBar(width: 180)
                }
                Text(" ").moneyFont(.caption).skeletonBar(width: 120, radius: 12)
            }
            VStack(alignment: .leading, spacing: HeroRhythm.withinChart) {
                CurveReadoutLine(readout: nil)
                SkeletonBlock(height: 160, radius: 12)
                MonacoRangeChipsSkeleton(ranges: LeaderboardRange.allCases)
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.sm)
        .padding(.bottom, MonacoTheme.Space.l)
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityElement()
        .accessibilityLabel("Loading your money in cabals")
        .accessibilityIdentifier("home-portfolio-loading")
    }
}
