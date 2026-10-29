import MonacoCore
import SwiftUI

struct HomePortfolioSkeleton: View {
    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            HomeFigureSkeleton()
            CurveScrubChartFrame { SkeletonBlock(height: nil, radius: MonacoTheme.Radius.cardTight) }
            MonacoRangeChipsSkeleton(ranges: LeaderboardRange.figurePeriods)
                .padding(.horizontal, MonacoTheme.Space.gutter)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityElement()
        .accessibilityLabel("Loading your net worth")
        .accessibilityIdentifier("home-portfolio-loading")
    }
}
