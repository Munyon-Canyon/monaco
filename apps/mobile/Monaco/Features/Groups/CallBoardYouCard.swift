import MonacoRanking
import SwiftUI

enum CallBoardCardMetrics {
    static let avatar: CGFloat = 40
    static let barHeight: CGFloat = 6
}

struct CallBoardYouCard: View {
    let card: CallBoardCard

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
            HStack(alignment: .top) {
                side(card.me, label: "You")
                VStack(spacing: MonacoTheme.Space.xs) {
                    Text(card.rankText).font(MonacoTheme.Typo.title).foregroundStyle(MonacoTheme.primaryText)
                    Text(card.ofLine).font(MonacoTheme.Typo.subhead).foregroundStyle(MonacoTheme.secondaryText)
                    if let line = card.neighbourLine {
                        Text(line).font(MonacoTheme.Typo.caption).foregroundStyle(MonacoTheme.secondaryText)
                    }
                }
                .frame(maxWidth: .infinity)
                if let neighbour = card.neighbour {
                    side(neighbour, label: nil)
                }
            }
            climbBar
            if let gap = card.gapLine {
                Text(gap).font(MonacoTheme.Typo.headline).foregroundStyle(MonacoTheme.primaryText)
            }
        }
        .padding(MonacoTheme.Space.m)
        .background(
            MonacoTheme.surface, in: RoundedRectangle(cornerRadius: MonacoTheme.Radius.card, style: .continuous)
        )
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.sm)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(
            [card.rankText, card.ofLine, card.neighbourLine, card.gapLine].compactMap { $0 }.joined(separator: ". ")
        )
        .accessibilityIdentifier("leaderboard-you-card")
    }

    private func side(_ row: CallBoardRowValue, label: String?) -> some View {
        VStack(spacing: MonacoTheme.Space.xs) {
            CallBoardAvatar(row: row, size: CallBoardCardMetrics.avatar)
            Text([label, row.rate].compactMap { $0 }.joined(separator: " · "))
                .font(MonacoTheme.Typo.headline).foregroundStyle(MonacoTheme.primaryText)
            Text(row.record).font(MonacoTheme.Typo.subhead).foregroundStyle(MonacoTheme.secondaryText)
        }
    }

    private var climbBar: some View {
        Capsule()
            .fill(MonacoTheme.border)
            .frame(height: CallBoardCardMetrics.barHeight)
            .overlay(alignment: .leading) {
                GeometryReader { proxy in
                    Capsule().fill(MonacoTheme.profit).frame(width: proxy.size.width * card.climb)
                }
            }
    }
}

struct CallBoardAvatar: View {
    let row: CallBoardRowValue
    let size: CGFloat

    var body: some View {
        if row.isCabal {
            CabalMark(groupId: row.id, name: row.name, size: size, pictureUrl: row.pictureURL)
        } else {
            MonacoAvatar(photoURL: row.pictureURL, displayName: row.name, size: size, seed: row.id)
        }
    }
}
