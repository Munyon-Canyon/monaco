import MonacoCore
import SwiftUI

struct CabalPauseRow: View {
    let pause: CabalPause

    var body: some View {
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
        .padding(MonacoTheme.Space.m)
        .background(MonacoTheme.goldWash, in: RoundedRectangle(cornerRadius: MonacoTheme.Radius.card))
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("cabal-pause-banner")
    }
}
