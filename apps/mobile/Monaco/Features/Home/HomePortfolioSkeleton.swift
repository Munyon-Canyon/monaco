import MonacoCore
import SwiftUI

enum HeroRhythm {
    static let titleToBalance = MonacoTheme.Space.xs
    static let balanceToChip = MonacoTheme.Space.s
    static let chipToChart = MonacoTheme.Space.m
    static let withinChart = MonacoTheme.Space.s
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
                CurveChangeLine(curve: nil, plot: .pnl)
                CurveScrubChartFrame { SkeletonBlock(height: nil, radius: 12) }
                MonacoRangeChipsSkeleton(ranges: LeaderboardRange.allCases)
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.sm)
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityElement()
        .accessibilityLabel("Loading your money in cabals")
        .accessibilityIdentifier("home-portfolio-loading")
    }
}
