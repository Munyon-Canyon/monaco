import SwiftUI

struct AssetDetailHeroSkeleton: View {
    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                SkeletonBlock(width: 104, height: 22, radius: 4)
                SkeletonBlock(width: 44, height: 13, radius: 3)
            }
            .padding(.vertical, 5)
            SkeletonBlock(width: 196, height: 36, radius: 6)
                .padding(.vertical, 8)
            HStack(spacing: MonacoTheme.Space.s) {
                SkeletonBlock(width: 118, height: 16, radius: 3)
                SkeletonBlock(width: 96, height: 12, radius: 3)
            }
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                SkeletonBlock(width: 124, height: 28, radius: 14)
                SkeletonBlock(width: 104, height: 12, radius: 3)
                    .padding(.vertical, 3)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("Loading stock")
        .accessibilityIdentifier("asset-detail-hero-skeleton")
    }
}
