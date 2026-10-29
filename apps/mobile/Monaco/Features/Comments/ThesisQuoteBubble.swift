import MonacoAPI
import SwiftUI

struct ThesisQuoteBubble: View {
    let commentID: String
    let attachment: Components.Schemas.ThesisQuoteAttachment

    @Environment(AppEnvironment.self) private var environment: AppEnvironment?
    @State private var expanded = false

    private static let collapsedLines = 6
    private static let longText = 240
    private static let minTap: CGFloat = 44

    private var isLong: Bool { attachment.text.unicodeScalars.count > Self.longText }
    private var header: String { "Thesis · \(attachment.proposerName)" }

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
            Button(action: open) {
                HStack(alignment: .top, spacing: MonacoTheme.Space.s) {
                    Rectangle()
                        .fill(MonacoTheme.brand)
                        .frame(width: MonacoTheme.Space.xs / 2)
                    VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                        Text(header)
                            .font(MonacoTheme.Typo.captionStrong)
                            .foregroundStyle(MonacoTheme.secondaryText)
                        Text(attachment.text)
                            .font(MonacoTheme.Typo.body)
                            .foregroundStyle(MonacoTheme.bodyText)
                            .lineLimit(expanded ? nil : Self.collapsedLines)
                            .multilineTextAlignment(.leading)
                    }
                    .frame(maxWidth: .infinity, alignment: .leading)
                }
                .frame(minHeight: Self.minTap, alignment: .leading)
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityElement(children: .combine)
            .accessibilityLabel("\(header). \(attachment.text)")
            .accessibilityHint("Opens the proposal")
            .accessibilityIdentifier("attachment-thesis-\(commentID)")
            if isLong, !expanded {
                Button("More") { expanded = true }
                    .font(MonacoTheme.Typo.captionStrong)
                    .foregroundStyle(MonacoTheme.brand)
                    .frame(minHeight: Self.minTap, alignment: .leading)
                    .accessibilityIdentifier("attachment-thesis-more-\(commentID)")
            }
        }
    }

    private func open() {
        guard let environment else { return }
        environment.navigator.open(
            ProposalRoute(proposalID: attachment.proposalId), in: environment.navigator.selectedTab)
    }
}
