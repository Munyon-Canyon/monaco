import Foundation
import MonacoAPI
import MonacoCore
import MonacoRanking
import SwiftUI

enum CallBoardSharePayload {
    static func make(standing: String, inviteURL: URL) -> SharePayload {
        SharePayload(title: standing, url: inviteURL)
    }
}

enum CallBoardDockedMetrics {
    static let avatar: CGFloat = 40
    static let barHeight: CGFloat = 6
    static let tap: CGFloat = 44
}

struct CallBoardDocked: View {
    let card: CallBoardCard
    var onShare: () -> Void

    var body: some View {
        HStack(spacing: MonacoTheme.Space.s) {
            summary
                .accessibilityElement(children: .combine)
                .accessibilityLabel(spoken)
                .accessibilityIdentifier("leaderboard-docked-row")
            if card.standing != nil {
                Button(action: onShare) {
                    Image(monaco: .share).foregroundStyle(MonacoTheme.profit)
                        .frame(width: CallBoardDockedMetrics.tap, height: CallBoardDockedMetrics.tap)
                }
                .accessibilityLabel(CallBoardCopy.shareStanding)
                .accessibilityIdentifier("leaderboard-docked-share")
            }
        }
        .padding(MonacoTheme.Space.m)
        .background(
            MonacoTheme.surface, in: RoundedRectangle(cornerRadius: MonacoTheme.Radius.card, style: .continuous)
        )
        .shadow(
            color: MonacoTheme.Shadow.floating.color, radius: MonacoTheme.Shadow.floating.radius, x: 0,
            y: MonacoTheme.Shadow.floating.y
        )
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.bottom, MonacoTheme.Space.s)
    }

    private var spoken: String { [card.me.spoken, card.gapLine].compactMap { $0 }.joined(separator: " ") }

    private var summary: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            HStack(spacing: MonacoTheme.Space.sm) {
                Text([card.rankText, card.me.movementGlyph].joined(separator: " "))
                    .font(MonacoTheme.Typo.headline).monospacedDigit().foregroundStyle(MonacoTheme.primaryText)
                CallBoardAvatar(row: card.me, size: CallBoardDockedMetrics.avatar)
                VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                    Text(card.me.name).font(MonacoTheme.Typo.headline).foregroundStyle(MonacoTheme.primaryText)
                    Text(card.me.turnout).font(MonacoTheme.Typo.subhead).foregroundStyle(MonacoTheme.secondaryText)
                }
                Spacer(minLength: 0)
                VStack(alignment: .trailing, spacing: MonacoTheme.Space.xs) {
                    Text(card.me.rate).font(MonacoTheme.Typo.headline).foregroundStyle(MonacoTheme.primaryText)
                    Text(card.me.record).font(MonacoTheme.Typo.subhead).foregroundStyle(MonacoTheme.secondaryText)
                }
            }
            Capsule().fill(MonacoTheme.border).frame(height: CallBoardDockedMetrics.barHeight)
                .overlay(alignment: .leading) {
                    GeometryReader { proxy in
                        Capsule().fill(MonacoTheme.profit).frame(width: proxy.size.width * card.climb)
                    }
                }
            if let gap = card.gapLine {
                Text(gap).font(MonacoTheme.Typo.subhead).foregroundStyle(MonacoTheme.primaryText)
            }
        }
    }
}
