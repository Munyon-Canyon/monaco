import SwiftUI

enum FeedAccentRuleMetrics {
    static let width: CGFloat = 3
    static let quoteWidth: CGFloat = 2
}

struct FeedAccentRule<Content: View>: View {
    let cabalID: String?
    @ViewBuilder let content: Content

    private var color: Color {
        guard let cabalID else { return MonacoTheme.border }
        return MonacoTheme.CabalTint.forGroupId(cabalID).fill
    }

    var body: some View {
        HStack(spacing: MonacoTheme.Space.sm) {
            Rectangle()
                .fill(color)
                .frame(width: FeedAccentRuleMetrics.width)
                .accessibilityHidden(true)
            content
                .frame(maxWidth: .infinity, alignment: .leading)
        }
        .fixedSize(horizontal: false, vertical: true)
    }
}
