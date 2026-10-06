import MonacoAPI
import MonacoCore
import SwiftUI

enum HomePortfolioSlot: HomeSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        HomePortfolioHero()
    }
}

private struct HomePortfolioHero: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(ScreenRefresh.self) private var refresh: ScreenRefresh?
    @State private var portfolio: PortfolioModel?
    @State private var chart: ValueChartModel?
    @State private var selection: Int?

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            content
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.sm)
        .padding(.bottom, MonacoTheme.Space.l)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(MonacoTheme.heroInk)
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
            skeleton
        case .failed:
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                title
                Text("Couldn't load your money in cabals.")
                    .font(MonacoTheme.Typo.callout)
                    .foregroundStyle(MonacoTheme.onHeroMuted)
                    .accessibilityIdentifier("home-portfolio-failed")
                Button("Try again") { retry() }
                    .buttonStyle(.monacoSecondary)
                    .accessibilityIdentifier("home-portfolio-retry")
            }
        case .loaded(let summary):
            title
            MoneyText(micros: summary.totalMicros, style: .hero, color: MonacoTheme.onHero)
                .accessibilityIdentifier("home-portfolio-total")
            chip(summary)
            if summary.isEmpty {
                hairline
            } else {
                curve
            }
        }
    }

    private var title: some View {
        Text("Your money in cabals")
            .font(MonacoTheme.Typo.captionStrong)
            .foregroundStyle(MonacoTheme.onHero)
            .accessibilityAddTraits(.isHeader)
    }

    private var skeleton: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            SkeletonBlock(width: 140, height: 14)
            SkeletonBlock(width: 180, height: 44)
            SkeletonBlock(width: 120, height: 24, radius: 12)
            SkeletonBlock(height: 160, radius: 12)
        }
        .accessibilityElement()
        .accessibilityLabel("Loading your money in cabals")
        .accessibilityIdentifier("home-portfolio-loading")
    }

    @ViewBuilder private func chip(_ summary: PortfolioSummary) -> some View {
        if summary.isEmpty {
            Text("\(summary.chip) · all time")
                .moneyFont(.caption, weight: .semibold)
                .foregroundStyle(MonacoTheme.onHeroMuted)
                .padding(.horizontal, 9)
                .padding(.vertical, 5)
                .background(Capsule().fill(MonacoTheme.onHeroHairline))
                .accessibilityIdentifier("home-portfolio-chip")
        } else {
            HStack(spacing: MonacoTheme.Space.s) {
                PnLBadge(dollarPnl: summary.pnl, percentReturn: summary.returnText, onInk: true)
                    .accessibilityIdentifier("home-portfolio-chip")
                Text("all time")
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.onHeroMuted)
            }
        }
    }

    private var hairline: some View {
        Rectangle()
            .fill(MonacoTheme.onHeroHairline)
            .frame(height: 1)
            .padding(.vertical, MonacoTheme.Space.m)
            .accessibilityHidden(true)
            .accessibilityIdentifier("home-portfolio-flat")
    }

    @ViewBuilder private var curve: some View {
        if let chart {
            switch chart.state {
            case .idle, .loading:
                SkeletonBlock(height: 160, radius: 12)
                    .accessibilityIdentifier("home-portfolio-chart-loading")
                rangeChips(chart)
            case .failed:
                Text("Couldn't load the chart.")
                    .font(MonacoTheme.Typo.callout)
                    .foregroundStyle(MonacoTheme.onHeroMuted)
                    .accessibilityIdentifier("home-portfolio-chart-failed")
                Button("Try again") { Task { await chart.load() } }
                    .buttonStyle(.monacoSecondary)
                    .accessibilityIdentifier("home-portfolio-chart-retry")
            case .loaded:
                if let curve = chart.curve, curve.hasEnoughHistory {
                    CurveReadoutLine(readout: selection.flatMap { curve.readout(at: $0) }, onInk: true)
                    CurveScrubChart(
                        curve: curve, range: chart.range, onInk: true, selection: $selection,
                        identifier: "home-pnl-chart")
                } else {
                    hairline
                    Text(chart.range.shortHistoryLine)
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.onHeroMuted)
                        .accessibilityIdentifier("home-portfolio-short")
                }
                rangeChips(chart)
            }
        }
    }

    private func rangeChips(_ chart: ValueChartModel) -> some View {
        MonacoRangeChips(
            ranges: chart.ranges, selection: chart.range, onInk: true, identifierPrefix: "home-portfolio"
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
