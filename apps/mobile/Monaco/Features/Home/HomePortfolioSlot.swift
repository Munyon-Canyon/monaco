import MonacoAPI
import MonacoCore
import SwiftUI

enum HomePortfolioSlot: HomeSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        HomePortfolioHero()
    }
}

enum HomeHeroMetrics {
    static let skeletonFigureWidth: CGFloat = 180
    static let skeletonReturnWidth: CGFloat = 140
    static let skeletonBarFraction: CGFloat = 0.43
}

struct HomeHeroFigures: Equatable {
    let amount: String
    let percent: String?
    let direction: PortfolioSummary.Direction
    let period: String

    var title: String { HomeHeroCopy.returnTitle(period: period) }
    var spokenTitle: String { HomeHeroCopy.spokenReturnTitle(period: period) }

    static func resolve(
        summary: PortfolioSummary, change: ValueCurve.Change?, range: LeaderboardRange
    ) -> HomeHeroFigures {
        guard let change else {
            return HomeHeroFigures(
                amount: summary.pnl, percent: summary.returnText == "—" ? nil : summary.returnText,
                direction: summary.direction, period: LeaderboardRange.all.returnPeriodPhrase)
        }
        return HomeHeroFigures(
            amount: change.amountText ?? UsdAmountFormatter.format(micros: 0), percent: change.percentText,
            direction: change.direction, period: range.returnPeriodPhrase)
    }
}

struct HomeHeroFigureText: View {
    let text: String
    var color: Color = MonacoTheme.ink
    var identifier = "home-portfolio-total"

    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    var body: some View {
        Text(text)
            .moneyFigure(.hero)
            .foregroundStyle(color)
            .lineLimit(1)
            .minimumScaleFactor(MoneyStyle.hero.minimumScaleFactor)
            .contentTransition(reduceMotion ? .identity : .numericText())
            .animation(reduceMotion ? nil : .snappy(duration: 0.25), value: text)
            .accessibilityIdentifier(identifier)
    }
}

struct HomeHeroSkeletonFigure: View {
    var body: some View {
        Text(UsdAmountFormatter.format(micros: 0))
            .moneyFont(.hero)
            .skeletonBar(width: HomeHeroMetrics.skeletonFigureWidth)
            .accessibilityHidden(true)
    }
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
            if reads?.plan.showsSharedRow == true {
                HomeSharedErrorState()
            } else {
                switch portfolio?.state ?? .loading {
                case .idle, .loading:
                    HomePageSkeleton()
                case .failed:
                    failed
                case .loaded(let summary):
                    loaded(summary)
                        .padding(.horizontal, MonacoTheme.Space.gutter)
                        .padding(.top, MonacoTheme.Space.m)
                        .frame(maxWidth: .infinity, alignment: .leading)
                }
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("home-portfolio")
        .onChange(of: HomeReadStatus(portfolio?.state ?? .loading), initial: true) { _, status in
            reads?.report(.portfolio, status)
        }
        .task {
            let chart = preparedChart()
            refresh?.register("home-portfolio-chart") { await chart.refresh() }
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

    @ViewBuilder private var failed: some View {
        if reads.showsOwnRow(.portfolio) {
            MonacoErrorRow(thing: "your portfolio", identifier: "home-portfolio-failed") {
                Task { await portfolio?.load() }
            }
            .padding(.top, MonacoTheme.Space.m)
        }
    }

    private var cashMicros: Int64 {
        if case .loaded(let balance) = environment.balance.state { return balance.availableMicros }
        return 0
    }

    @ViewBuilder private func loaded(_ summary: PortfolioSummary) -> some View {
        let variant = HomeVariant(summary: summary, cashMicros: cashMicros)
        let display = HomeBalanceDisplay.resolve(environment.balance.state)
        VStack(alignment: .leading, spacing: 0) {
            stale
            switch variant {
            case .invested: invested(summary)
            case .cashOnly: HomeCashOnlyHero(display: display)
            case .empty:
                if display == .loading { figuresSkeleton } else { HomeNoHistoryLine() }
            }
        }
    }

    @ViewBuilder private var stale: some View {
        if portfolio?.isStale == true {
            TimelineView(.periodic(from: .now, by: 60)) { context in
                MonacoNotice(
                    .warn, title: PortfolioStaleCopy.title,
                    message: PortfolioStaleCopy.line(lastUpdated: portfolio?.lastUpdated, now: context.date)
                )
                .padding(.bottom, MonacoTheme.Space.m)
            }
            .accessibilityIdentifier("home-portfolio-stale")
        }
    }

    private func figure(_ text: String) -> some View {
        HomeHeroFigureText(text: text)
    }

    @ViewBuilder private func invested(_ summary: PortfolioSummary) -> some View {
        if let chart {
            VStack(alignment: .leading, spacing: 0) {
                investedRows(summary, chart)
            }
        } else {
            VStack(alignment: .leading, spacing: 0) {
                figuresSkeleton
                chartSkeleton
            }
        }
    }

    @ViewBuilder private func investedRows(_ summary: PortfolioSummary, _ chart: ValueChartModel) -> some View {
        switch chart.state {
        case .idle, .loading:
            figuresSkeleton
            chartSkeleton
            MonacoRangeChipsSkeleton(ranges: LeaderboardRange.homePeriods)
        case .failed:
            figures(HomeHeroFigures.resolve(summary: summary, change: nil, range: chart.range))
            MonacoErrorRow(thing: "the chart", identifier: "home-portfolio-chart-failed", inset: false) {
                Task { await chart.load() }
            }
            .padding(.top, MonacoTheme.Space.m)
        case .loaded:
            let shown = chart.shownRange ?? chart.range
            let curve = chart.curve.flatMap { $0.hasEnoughHistory ? $0 : nil }
            figures(HomeHeroFigures.resolve(summary: summary, change: change(curve), range: shown))
            plot(curve, range: shown)
                .padding(.top, MonacoTheme.Space.m)
            periods(chart)
        }
    }

    private func change(_ curve: ValueCurve?) -> ValueCurve.Change? {
        guard let curve else { return nil }
        guard let selection else { return curve.change(.pnl) }
        return curve.change(.pnl, to: min(selection, max(curve.plotted(.pnl).count - 1, 0)))
    }

    private func color(_ direction: PortfolioSummary.Direction) -> Color {
        switch direction {
        case .up: MonacoTheme.profit
        case .down: MonacoTheme.loss
        case .flat: MonacoTheme.muted
        }
    }

    @ViewBuilder private func figures(_ figures: HomeHeroFigures) -> some View {
        Text(figures.title)
            .font(MonacoTheme.Typo.caption)
            .foregroundStyle(MonacoTheme.muted)
            .fixedSize(horizontal: false, vertical: true)
            .accessibilityLabel(figures.spokenTitle)
            .accessibilityIdentifier("home-portfolio-label")
        figure(figures.amount)
            .padding(.top, MonacoTheme.Space.xs)
        returnChip(figures)
            .padding(.top, MonacoTheme.Space.xs)
    }

    @ViewBuilder private func returnChip(_ figures: HomeHeroFigures) -> some View {
        Group {
            if let percent = figures.percent {
                Text(percent)
                    .moneyFigure(.row)
                    .foregroundStyle(color(figures.direction))
                    .lineLimit(1)
                    .fixedSize()
                    .accessibilityLabel("\(PnLSpeech.percent(percent)), \(figures.period)")
                    .accessibilityValue(percent)
            } else {
                Text(HomeHeroCopy.noReturn)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
            }
        }
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("home-portfolio-chip")
    }

    private var figuresSkeleton: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
            Text(" ").font(MonacoTheme.Typo.caption)
                .skeletonBar(width: HomeHeroMetrics.skeletonReturnWidth, radius: MonacoTheme.Radius.chip)
            HomeHeroSkeletonFigure()
            Text(" ").moneyFont(.row)
                .skeletonBar(width: HomeHeroMetrics.skeletonReturnWidth, radius: MonacoTheme.Radius.chip)
        }
        .accessibilityHidden(true)
    }

    private var chartSkeleton: some View {
        CurveScrubChartFrame { SkeletonBlock(height: nil, radius: MonacoTheme.Radius.cardTight) }
            .padding(.top, MonacoTheme.Space.m)
            .accessibilityElement(children: .ignore)
            .accessibilityLabel("Loading the chart")
            .accessibilityIdentifier("home-portfolio-chart-loading")
    }

    @ViewBuilder private func plot(_ curve: ValueCurve?, range: LeaderboardRange) -> some View {
        if let curve {
            CurveScrubChart(
                curve: curve, plot: .pnl, range: range, selection: $selection, identifier: "home-pnl-chart")
        } else {
            CurveScrubChartFrame {
                Text(range.shortHistoryLine)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
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

    private func preparedChart() -> ValueChartModel {
        if let chart { return chart }
        let chart = ValueChartModel(
            subjects: [.me], ranges: LeaderboardRange.homePeriods, range: .oneMonth, api: environment.api,
            hints: environment.hints)
        self.chart = chart
        return chart
    }
}

struct HomePageSkeleton: View {
    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                SkeletonTitle()
                SkeletonFraction(HomeHeroMetrics.skeletonBarFraction) { SkeletonBar() }
                    .frame(maxWidth: .infinity, alignment: .leading)
                SkeletonCardBlock()
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            MonacoRowSkeleton(rows: 4, markShape: .circle, hasTrailing: false)
        }
        .padding(.top, MonacoTheme.Space.m)
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityElement()
        .accessibilityLabel("Loading Home")
        .accessibilityIdentifier("home-portfolio-loading")
    }
}
