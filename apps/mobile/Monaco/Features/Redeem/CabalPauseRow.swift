import MonacoCore
import SwiftUI

struct CabalPauseRow: View {
    let pause: CabalPause

    var body: some View {
        VStack(spacing: 0) {
            MonacoRule()
            HStack(alignment: .firstTextBaseline, spacing: MonacoTheme.Space.sm) {
                Image(systemName: "pause.circle.fill")
                    .font(.body)
                    .foregroundStyle(MonacoTheme.warning)
                    .accessibilityHidden(true)
                Text(pause.message)
                    .font(MonacoTheme.Typo.callout)
                    .foregroundStyle(MonacoTheme.ink)
                    .fixedSize(horizontal: false, vertical: true)
                    .frame(maxWidth: .infinity, alignment: .leading)
            }
            .frame(minHeight: MonacoRowLayout.minHeight)
            MonacoRule()
        }
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("cabal-pause-banner")
    }
}
