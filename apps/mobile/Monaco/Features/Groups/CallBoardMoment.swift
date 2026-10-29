import MonacoCore
import MonacoRanking
import SwiftUI

enum CallBoardMomentMetrics {
    static let avatar: CGFloat = 40
}

struct CallBoardMomentCard: View {
    let moment: CallBoardMoment

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        NavigationLink(value: route) {
            Group {
                if dynamicTypeSize.isAccessibilitySize {
                    VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                        HStack(spacing: MonacoTheme.Space.sm) {
                            leader
                            texts
                        }
                        glyph
                    }
                    .frame(maxWidth: .infinity, alignment: .leading)
                } else {
                    HStack(spacing: MonacoTheme.Space.sm) {
                        leader
                        texts
                        Spacer(minLength: 0)
                        glyph
                    }
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

    private var leader: some View {
        VStack(spacing: MonacoTheme.Space.xs) {
            Image(systemName: "crown.fill").foregroundStyle(MonacoTheme.crown).accessibilityHidden(true)
            avatar
        }
    }

    private var texts: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
            Text(moment.title).font(MonacoTheme.Typo.headline).foregroundStyle(MonacoTheme.primaryText)
            Text(moment.subline).font(MonacoTheme.Typo.subhead).foregroundStyle(MonacoTheme.secondaryText)
        }
    }

    @ViewBuilder private var glyph: some View {
        if let glyph = moment.glyph {
            Text(glyph).font(MonacoTheme.Typo.headline).monospacedDigit().foregroundStyle(MonacoTheme.profit)
        }
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
