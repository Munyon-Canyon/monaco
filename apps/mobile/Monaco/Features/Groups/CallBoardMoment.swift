import MonacoCore
import MonacoRanking
import SwiftUI

enum CallBoardMomentMetrics {
    static let avatar: CGFloat = 40
}

struct CallBoardMomentCard: View {
    let moment: CallBoardMoment

    var body: some View {
        NavigationLink(value: route) {
            HStack(spacing: MonacoTheme.Space.sm) {
                VStack(spacing: MonacoTheme.Space.xs) {
                    Image(systemName: "crown.fill").foregroundStyle(MonacoTheme.crown).accessibilityHidden(true)
                    avatar
                }
                VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                    Text(moment.title).font(MonacoTheme.Typo.headline).foregroundStyle(MonacoTheme.primaryText)
                    Text(moment.subline).font(MonacoTheme.Typo.subhead).foregroundStyle(MonacoTheme.secondaryText)
                }
                Spacer(minLength: 0)
                if let glyph = moment.glyph {
                    Text(glyph).font(MonacoTheme.Typo.headline).monospacedDigit().foregroundStyle(MonacoTheme.profit)
                }
            }
            .padding(MonacoTheme.Space.m)
            .background(
                MonacoTheme.surface, in: RoundedRectangle(cornerRadius: MonacoTheme.Radius.card, style: .continuous)
            )
        }
        .buttonStyle(.plain)
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.sm)
        .accessibilityElement(children: .combine)
        .accessibilityLabel(moment.spoken)
        .accessibilityAddTraits(.isButton)
        .accessibilityIdentifier("leaderboard-lead-change")
    }

    @ViewBuilder private var avatar: some View {
        if moment.leaderIsCabal {
            CabalMark(
                groupId: moment.leaderID, name: moment.leaderName, size: CallBoardMomentMetrics.avatar,
                pictureUrl: moment.leaderPictureURL)
        } else {
            MonacoAvatar(
                photoURL: moment.leaderPictureURL, displayName: moment.leaderName,
                size: CallBoardMomentMetrics.avatar, seed: moment.leaderID)
        }
    }

    private var route: AnyAppRoute {
        moment.leaderIsCabal
            ? AnyAppRoute(CabalRoute(id: moment.leaderID))
            : AnyAppRoute(
                UserProfileRoute(
                    userID: moment.leaderID,
                    preview: UserPreview(
                        displayName: moment.leaderName, handle: moment.leaderHandle,
                        photoURL: moment.leaderPictureURL)))
    }
}
