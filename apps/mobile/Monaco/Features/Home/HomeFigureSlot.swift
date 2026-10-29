import MonacoAPI
import MonacoCore
import SwiftUI

enum HomeFigureSlot: HomeSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        HomeFigureView()
    }
}

enum HomeFigureMetrics {
    static let skeletonFigureWidth: CGFloat = 180
    static let skeletonLineWidth: CGFloat = 140
}

struct HomeFigureView: View {
    @Environment(PortfolioModel.self) private var portfolio: PortfolioModel?
    @Environment(\.homeReads) private var reads

    var body: some View {
        Group {
            if reads?.plan.showsSharedRow == true {
                HomeSharedErrorState()
            } else {
                switch portfolio?.state ?? .loading {
                case .idle, .loading: HomeFigureSkeleton()
                case .failed: failed
                case .loaded: loaded
                }
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("home-portfolio")
        .onChange(of: HomeReadStatus(portfolio?.state ?? .loading), initial: true) { _, status in
            reads?.report(.portfolio, status)
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

    @ViewBuilder private var loaded: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
            stale
            if let netWorth = portfolio?.netWorth {
                figure(netWorth)
                line(netWorth)
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.m)
        .frame(maxWidth: .infinity, alignment: .leading)
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

    private func figure(_ micros: Int64) -> some View {
        let parts = HomeFigureCopy.netWorth(micros: micros)
        return HStack(alignment: .firstTextBaseline, spacing: 0) {
            Text(parts.whole).foregroundStyle(MonacoTheme.ink)
            Text(parts.cents).foregroundStyle(MonacoTheme.secondaryText)
        }
        .moneyFigure(.home)
        .lineLimit(1)
        .minimumScaleFactor(MoneyStyle.home.minimumScaleFactor)
        .accessibilityIdentifier("home-portfolio-total")
    }

    @ViewBuilder private func line(_ netWorth: Int64) -> some View {
        if let portfolio, let pnl = portfolio.pnl, let bps = portfolio.returnBps {
            let range = portfolio.shownRange
            let copy = HomeFigureCopy.line(pnlMicros: pnl, returnBps: bps, range: range)
            HStack(spacing: MonacoTheme.Space.xs) {
                Text("\(copy.amount) \u{00B7} \(copy.percent)").foregroundStyle(color(copy.direction))
                Text(copy.words).foregroundStyle(MonacoTheme.secondaryText)
            }
            .font(MonacoTheme.Typo.subhead)
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(
                HomeFigureCopy.spoken(netWorthMicros: netWorth, pnlMicros: pnl, returnBps: bps, range: range)
            )
            .accessibilityIdentifier("home-portfolio-chip")
        }
    }

    private func color(_ direction: PortfolioSummary.Direction) -> Color {
        switch direction {
        case .up: MonacoTheme.profit
        case .down: MonacoTheme.loss
        case .flat: MonacoTheme.muted
        }
    }
}

struct HomeFigureSkeleton: View {
    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
            Text(UsdAmountFormatter.format(micros: 0))
                .moneyFigure(.home)
                .skeletonBar(width: HomeFigureMetrics.skeletonFigureWidth)
            Text(" ").font(MonacoTheme.Typo.subhead)
                .skeletonBar(width: HomeFigureMetrics.skeletonLineWidth, radius: MonacoTheme.Radius.chip)
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.m)
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityElement()
        .accessibilityLabel("Loading your net worth")
        .accessibilityIdentifier("home-portfolio-loading")
    }
}
