import SwiftUI

struct ProposalReasonSection: View {
    enum Style {
        case page
        case sheet
        case plain
    }

    let title: String
    let text: String
    var style: Style = .page

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            if style != .plain { MonacoSectionHeader(title) }
            Text(text)
                .font(style == .sheet ? MonacoTheme.Typo.subhead : MonacoTheme.Typo.body)
                .lineSpacing(style == .sheet ? 2 : 4)
                .foregroundStyle(MonacoTheme.bodyText)
                .fixedSize(horizontal: false, vertical: true)
                .frame(maxWidth: .infinity, alignment: .leading)
                .accessibilityIdentifier("proposal-reason")
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
    }
}
