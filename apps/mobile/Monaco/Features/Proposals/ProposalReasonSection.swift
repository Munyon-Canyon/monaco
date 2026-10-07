import SwiftUI

struct ProposalReasonSection: View {
    let title: String
    let text: String

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            MonacoSectionHeader(title)
                .padding(.horizontal, MonacoTheme.Space.gutter)
            MonacoGroupedList {
                Text(text)
                    .font(MonacoTheme.Typo.body)
                    .foregroundStyle(MonacoTheme.ink)
                    .fixedSize(horizontal: false, vertical: true)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                    .padding(.vertical, MonacoTheme.Space.m)
                    .accessibilityIdentifier("proposal-reason")
            }
        }
    }
}
