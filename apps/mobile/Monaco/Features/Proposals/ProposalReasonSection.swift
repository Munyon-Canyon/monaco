import SwiftUI

struct ProposalReasonSection: View {
    enum Style {
        case page
        case sheet
    }

    let title: String
    let text: String
    var style: Style = .page

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            MonacoSectionHeader(title)
            Text(text)
                .font(style == .page ? MonacoTheme.Typo.body : MonacoTheme.Typo.subhead)
                .lineSpacing(style == .page ? 4 : 2)
                .foregroundStyle(MonacoTheme.bodyText)
                .fixedSize(horizontal: false, vertical: true)
                .frame(maxWidth: .infinity, alignment: .leading)
                .accessibilityIdentifier("proposal-reason")
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
    }
}
