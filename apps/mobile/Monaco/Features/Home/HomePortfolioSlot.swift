import SwiftUI

enum HomePortfolioSlot: HomeSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
            Text("Your money in cabals")
                .font(MonacoTheme.Typo.captionStrong)
                .foregroundStyle(MonacoTheme.onHero)
            Text("Your total shows up here soon.")
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.onHeroMuted)
                .accessibilityIdentifier("home-portfolio-coming")
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.sm)
        .padding(.bottom, MonacoTheme.Space.l)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(MonacoTheme.heroInk)
    }
}
