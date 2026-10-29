import MonacoAPI
import MonacoCore
import SwiftUI

struct CatchUpCaughtUp: View {
    let screen: CatchUpScreen
    let openFeed: () -> Void

    private static let crestSize: CGFloat = 72

    var body: some View {
        VStack(spacing: MonacoTheme.Space.m) {
            if let first = screen.cabals.first {
                CabalMark(groupId: first.id, name: first.name, size: Self.crestSize, pictureUrl: first.pictureUrl)
                    .accessibilityHidden(true)
            }
            Text(CatchUpCopy.caughtUpTitle)
                .font(MonacoTheme.Typo.title)
                .foregroundStyle(MonacoTheme.ink)
            Text(CatchUpCopy.caughtUpLine(cabals: screen.cabals))
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.muted)
                .multilineTextAlignment(.center)
            Button(action: openFeed) {
                Label(CatchUpCopy.openFeed, systemImage: "newspaper")
            }
            .buttonStyle(.monacoSecondary)
            .accessibilityIdentifier("catch-up-open-feed")
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.vertical, MonacoTheme.Space.l)
        .frame(maxWidth: .infinity)
        .accessibilityElement(children: .contain)
    }
}
