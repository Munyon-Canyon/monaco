import SwiftUI

struct AssetDetailHeroSkeleton: View {
    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            SkeletonTitle()
            SkeletonFraction(SkeletonMetrics.rowSubtitleFraction) { SkeletonBar() }
            SkeletonCardBlock()
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("Loading stock")
        .accessibilityIdentifier("asset-detail-hero-skeleton")
    }
}
