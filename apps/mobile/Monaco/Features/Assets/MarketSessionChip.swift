import MonacoCore
import SwiftUI

struct MarketSessionChip: View {
    let session: MarketSessionChipCopy

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
            StatusBadge(session.title, tone: session.isLive ? .neutral : .warn)
            if let detail = session.detail {
                Text(detail)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .fixedSize(horizontal: false, vertical: true)
            }
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(session.spoken)
    }
}
