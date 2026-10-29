import MonacoAPI
import MonacoCore
import SwiftUI

struct CatchUpCaughtUp: View {
    let cabals: [Components.Schemas.CatchUpCabal]
    let openFeed: () -> Void

    var body: some View {
        VStack(spacing: MonacoTheme.Space.m) {
            if let cabal = cabals.first {
                CabalCover(
                    cabalId: cabal.id, tint: .forGroupId(cabal.id), pictureURL: CabalMark.resolvedURL(cabal.pictureUrl),
                    style: .thumb(CabalMark.Size.header.rawValue)
                )
                .frame(width: CabalMark.Size.header.rawValue, height: CabalMark.Size.header.rawValue)
                .accessibilityHidden(true)
            }
            Text(CatchUpCopy.caughtUpTitle)
                .font(MonacoTheme.Typo.title2)
                .foregroundStyle(MonacoTheme.ink)
                .accessibilityAddTraits(.isHeader)
            Text(CatchUpCopy.caughtUpLine(cabals: cabals))
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.muted)
                .multilineTextAlignment(.center)
                .accessibilityIdentifier("catch-up-caught-up-line")
            Button(action: openFeed) {
                Label(CatchUpCopy.openFeed, systemImage: MainTab.feed.systemImage)
            }
            .buttonStyle(.monacoSecondary)
            .accessibilityIdentifier("catch-up-open-feed")
        }
        .frame(maxWidth: .infinity)
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.l)
        .padding(.bottom, MonacoTheme.Space.l)
    }
}
