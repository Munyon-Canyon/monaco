import MonacoCore
import SwiftUI

struct CabalPauseRow: View {
    let pause: CabalPause

    var body: some View {
        WarningNoticeRow(message: pause.message, identifier: "cabal-pause-banner")
    }
}

struct WarningNoticeRow: View {
    let message: String
    let identifier: String
    var systemImage = "pause.circle.fill"

    var body: some View {
        VStack(spacing: 0) {
            MonacoRule()
            HStack(alignment: .firstTextBaseline, spacing: MonacoTheme.Space.sm) {
                Image(systemName: systemImage)
                    .font(.body)
                    .foregroundStyle(MonacoTheme.warning)
                    .accessibilityHidden(true)
                Text(message)
                    .font(MonacoTheme.Typo.callout)
                    .foregroundStyle(MonacoTheme.ink)
                    .fixedSize(horizontal: false, vertical: true)
                    .frame(maxWidth: .infinity, alignment: .leading)
            }
            .frame(minHeight: MonacoRowLayout.minHeight)
            MonacoRule()
        }
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier(identifier)
    }
}
