import MonacoRanking
import SwiftUI

enum CallBoardCardMetrics {
    static let avatar: CGFloat = 64
    static let barHeight: CGFloat = 6
}

struct CallBoardYouCard: View {
    let card: CallBoardCard

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
            if dynamicTypeSize.isAccessibilitySize {
                VStack(spacing: MonacoTheme.Space.sm) {
                    side(card.me, label: card.meLabel)
                    middle
                    if let neighbour = card.neighbour {
                        side(neighbour, label: nil)
                    }
                }
                .frame(maxWidth: .infinity)
            } else {
                HStack(alignment: .top) {
                    side(card.me, label: card.meLabel)
                    middle
                        .frame(maxWidth: .infinity)
                    if let neighbour = card.neighbour {
                        side(neighbour, label: nil)
                    }
                }
            }
            CallBoardClimbBar(fraction: card.climb)
            if let gap = card.gapLine {
                Text(gap).font(MonacoTheme.Typo.subhead).foregroundStyle(MonacoTheme.primaryText)
                    .multilineTextAlignment(.center)
                    .frame(maxWidth: .infinity)
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
            (cabalNames + [card.rankText, card.ofLine, card.neighbourLine, card.gapLine]).compactMap { $0 }
                .joined(separator: ". ")
        )
        .accessibilityIdentifier("leaderboard-you-card")
    }

    private var middle: some View {
        VStack(spacing: MonacoTheme.Space.xs) {
            Text(card.rankText).font(MonacoTheme.Typo.title).foregroundStyle(MonacoTheme.secondaryText)
            Text(card.ofLine).font(MonacoTheme.Typo.subhead).foregroundStyle(MonacoTheme.secondaryText)
            if let line = card.neighbourLine {
                Text(line).font(MonacoTheme.Typo.caption).foregroundStyle(MonacoTheme.secondaryText)
            }
        }
    }

    private var cabalNames: [String?] {
        [card.me, card.neighbour].compactMap { $0 }.filter(\.isCabal).map(\.name)
    }

    private func side(_ row: CallBoardRowValue, label: String?) -> some View {
        VStack(spacing: MonacoTheme.Space.xs) {
            CallBoardAvatar(row: row, size: CallBoardCardMetrics.avatar)
            Text(label ?? row.name).font(MonacoTheme.Typo.headline).foregroundStyle(MonacoTheme.primaryText)
            Text(row.cardStat).font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.secondaryText)
                .multilineTextAlignment(.center)
                .lineLimit(1)
                .minimumScaleFactor(0.8)
        }
    }
}

struct CallBoardClimbBar: View {
    let fraction: Double

    var body: some View {
        Capsule()
            .fill(MonacoTheme.border)
            .frame(height: CallBoardCardMetrics.barHeight)
            .overlay(alignment: .leading) {
                GeometryReader { proxy in
                    Capsule().fill(MonacoTheme.profit).frame(width: proxy.size.width * fraction)
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
