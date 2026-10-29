import MonacoRanking
import SwiftUI

struct CallBoardUnrankedCardView: View {
    let card: CallBoardUnrankedCard

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
            HStack(spacing: MonacoTheme.Space.sm) {
                CallBoardAvatar(row: card.subject, size: CallBoardCardMetrics.avatar)
                VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                    Text(card.title).font(MonacoTheme.Typo.headline).foregroundStyle(MonacoTheme.primaryText)
                    Text(card.progress).font(MonacoTheme.Typo.subhead).foregroundStyle(MonacoTheme.secondaryText)
                }
            }
            CallBoardClimbBar(fraction: card.climb)
            Text(card.pending).font(MonacoTheme.Typo.subhead).foregroundStyle(MonacoTheme.secondaryText)
        }
        .padding(MonacoTheme.Space.m)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(
            MonacoTheme.surface, in: RoundedRectangle(cornerRadius: MonacoTheme.Radius.card, style: .continuous)
        )
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.sm)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel([card.title, card.progress, card.pending].joined(separator: ". "))
        .accessibilityIdentifier("leaderboard-unranked-card")
    }
}
