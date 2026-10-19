import MonacoCore
import SwiftUI

struct SocialCountsModel: Equatable {
    var userID: String
    var followers: Int
    var following: Int
    var cabals: Int?
}

struct SocialCounts: View {
    let model: SocialCountsModel
    var identifierPrefix = "profile"

    var body: some View {
        ViewThatFits(in: .horizontal) {
            HStack(spacing: MonacoTheme.Space.s) {
                followersItem
                Spacer(minLength: MonacoTheme.Space.s)
                followingItem
                if model.cabals != nil {
                    Spacer(minLength: MonacoTheme.Space.s)
                    cabalsItem
                }
            }
            VStack(alignment: .leading, spacing: 0) {
                followersItem
                followingItem
                cabalsItem
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(.horizontal, MonacoTheme.Space.gutter)
    }

    private var followersItem: some View {
        link(
            FollowCountFormatter.count(model.followers), FollowCountFormatter.followersNoun(model.followers),
            .followers, id: "\(identifierPrefix)-followers")
    }

    private var followingItem: some View {
        link(FollowCountFormatter.count(model.following), "following", .following, id: "\(identifierPrefix)-following")
    }

    @ViewBuilder private var cabalsItem: some View {
        if let cabals = model.cabals {
            item(FollowCountFormatter.count(cabals), cabals == 1 ? "cabal" : "cabals")
                .accessibilityIdentifier("\(identifierPrefix)-stat-cabals")
        }
    }

    private func link(_ count: String, _ noun: String, _ kind: FollowListKind, id: String) -> some View {
        NavigationLink(value: AnyAppRoute(FollowListRoute(userID: model.userID, kind: kind))) {
            item(count, noun)
        }
        .buttonStyle(.plain)
        .accessibilityIdentifier(id)
    }

    private func item(_ count: String, _ noun: String) -> some View {
        HStack(spacing: MonacoTheme.Space.xs) {
            Text(count)
                .font(MonacoTheme.Typo.captionStrong)
                .foregroundStyle(MonacoTheme.primaryText)
            Text(noun)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
        }
        .fixedSize(horizontal: false, vertical: true)
        .frame(minHeight: MonacoButtonMetrics.hitTarget)
        .contentShape(Rectangle())
        .accessibilityElement(children: .combine)
    }
}
