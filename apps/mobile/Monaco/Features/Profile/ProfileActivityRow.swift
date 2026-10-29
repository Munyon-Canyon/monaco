import MonacoCore
import SwiftUI

struct ProfileActivityRowView: View {
    let row: MemberActivityRow
    let index: Int
    let isLast: Bool
    var identifierPrefix = "profile"

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    private var route: AnyAppRoute {
        switch row.destination {
        case .proposal(let id): AnyAppRoute(ProposalRoute(proposalID: id))
        case .board: AnyAppRoute(LeaderboardRoute(board: .people, range: .oneMonth))
        }
    }

    var body: some View {
        NavigationLink(value: route) {
            HStack(alignment: dynamicTypeSize.isAccessibilitySize ? .top : .center, spacing: MonacoTheme.Space.sm) {
                Image(systemName: row.glyph.symbol)
                    .font(MonacoTheme.Typo.headline)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .frame(width: 24, height: 24)
                    .accessibilityHidden(true)
                VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                    Text(row.title)
                        .font(MonacoTheme.Typo.rowTitle)
                        .foregroundStyle(MonacoTheme.primaryText)
                        .fixedSize(horizontal: false, vertical: true)
                    Text(row.subtitle)
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.secondaryText)
                        .fixedSize(horizontal: false, vertical: true)
                    if dynamicTypeSize.isAccessibilitySize { timeText }
                }
                .frame(maxWidth: .infinity, alignment: .leading)
                if !dynamicTypeSize.isAccessibilitySize { timeText }
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.vertical, MonacoTheme.Space.s)
            .frame(minHeight: 44)
            .contentShape(Rectangle())
            .overlay(alignment: .bottom) {
                if !isLast {
                    MonacoRule()
                        .padding(.leading, MonacoTheme.Space.sm + 24)
                        .padding(.horizontal, MonacoTheme.Space.gutter)
                }
            }
        }
        .buttonStyle(.monacoRow)
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("\(identifierPrefix)-activity-row-\(index)")
    }

    private var timeText: some View {
        Text(row.time)
            .font(MonacoTheme.Typo.caption)
            .foregroundStyle(MonacoTheme.secondaryText)
    }
}
