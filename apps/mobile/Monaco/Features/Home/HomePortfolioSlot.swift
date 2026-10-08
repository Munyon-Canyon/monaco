import MonacoAPI
import MonacoCore
import SwiftUI

enum HomePortfolioSlot: HomeSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        HomePortfolioHero()
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
    @State private var portfolio: PortfolioModel?
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
        .task {
            let (portfolio, chart) = preparedModels()
            refresh?.register("home-portfolio") { await reload(portfolio, chart) }
            await withTaskGroup(of: Void.self) { group in
                group.addTask { await reload(portfolio, chart) }
                group.addTask { await portfolio.observe() }
                group.addTask { await chart.observe() }
            }
        }
        .onScreenVisibilityChange { visible in
            portfolio?.setVisible(visible)
            chart?.setVisible(visible)
        }
        .onChange(of: portfolio?.toast) { _, message in
            guard let message else { return }
            toasts.current = MonacoToast(message: message)
            portfolio?.dismissToast()
        }
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
                title
                MonacoErrorRow(thing: "your money in cabals", identifier: "home-portfolio-failed") {
                    retry()
                }
            }
        case .loaded(let summary):
            VStack(alignment: .leading, spacing: HeroRhythm.chipToChart) {
                VStack(alignment: .leading, spacing: HeroRhythm.balanceToChip) {
                    VStack(alignment: .leading, spacing: HeroRhythm.titleToBalance) {
                        title
                        MoneyText(micros: summary.totalMicros, style: .hero)
                            .accessibilityIdentifier("home-portfolio-total")
                    }
                    chip(summary)
                }
                if summary.isEmpty {
                    hairline
                } else {
                    curve
                }
            }
        }
    }

    private var title: some View {
        Text("Your money in cabals")
            .font(MonacoTheme.Typo.callout)
            .foregroundStyle(MonacoTheme.muted)
            .accessibilityAddTraits(.isHeader)
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
            SkeletonBlock(height: 160, radius: 12)
                .accessibilityIdentifier("home-portfolio-chart-loading")
            rangeChips(chart)
        case .failed:
            MonacoErrorRow(thing: "the chart", identifier: "home-portfolio-chart-failed") {
                Task { await chart.load() }
            }
        case .loaded:
            if let curve = chart.curve, curve.hasEnoughHistory {
                CurveReadoutLine(readout: selection.flatMap { curve.readout(at: $0) })
                CurveScrubChart(
                    curve: curve, range: chart.range, selection: $selection,
                    identifier: "home-pnl-chart")
            } else {
                hairline
                Text(chart.range.shortHistoryLine)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
                    .accessibilityIdentifier("home-portfolio-short")
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

    private func retry() {
        guard let portfolio, let chart else { return }
        Task { await reload(portfolio, chart) }
    }

    private func reload(_ portfolio: PortfolioModel, _ chart: ValueChartModel) async {
        await withTaskGroup(of: Void.self) { group in
            group.addTask { await portfolio.load() }
            group.addTask { await chart.load() }
        }
    }

    private func preparedModels() -> (PortfolioModel, ValueChartModel) {
        if let portfolio, let chart { return (portfolio, chart) }
        let portfolio = PortfolioModel(api: environment.api, hints: environment.hints)
        let chart = ValueChartModel(
            subjects: [.me], ranges: LeaderboardRange.allCases, range: .oneDay, api: environment.api,
            hints: environment.hints)
        self.portfolio = portfolio
        self.chart = chart
        return (portfolio, chart)
    }
}

struct HomePortfolioSkeleton: View {
    var body: some View {
        VStack(alignment: .leading, spacing: HeroRhythm.chipToChart) {
            VStack(alignment: .leading, spacing: HeroRhythm.balanceToChip) {
                VStack(alignment: .leading, spacing: HeroRhythm.titleToBalance) {
                    SkeletonBlock(width: 140, height: 16)
                    SkeletonBlock(width: 180, height: 44)
                }
                SkeletonBlock(width: 120, height: 24, radius: 12)
            }
            SkeletonBlock(height: 160, radius: 12)
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
