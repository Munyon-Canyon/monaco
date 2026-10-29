import MonacoCore
import SwiftUI

enum ProfileStandingSlot: ProfileSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        ProfileStanding()
    }
}

private struct ProfileStanding: View {
    @Environment(OwnProfileModel.self) private var own: OwnProfileModel?

    var body: some View {
        if let own, let counts = own.counts {
            VStack(alignment: .leading, spacing: 0) {
                ProfileCountsLine(userID: own.userID, counts: counts)
                if let standing = own.standing {
                    NavigationLink(value: AnyAppRoute(LeaderboardRoute(board: .people, range: .oneMonth))) {
                        ProfileCopyText(line: standing.line)
                            .frame(minHeight: MonacoButtonMetrics.hitTarget, alignment: .leading)
                            .contentShape(Rectangle())
                    }
                    .buttonStyle(.plain)
                    .accessibilityLabel(standing.line.plain)
                    .accessibilityHint("Opens the month leaderboard")
                    .accessibilityIdentifier("profile-standing")
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.top, MonacoTheme.Space.s - SectionStackMetrics.spacing)
        }
    }
}

private struct ProfileCountsLine: View {
    let userID: String
    let counts: OwnProfileCounts

    var body: some View {
        ViewThatFits(in: .horizontal) {
            HStack(spacing: 0) { items(separated: true) }
            VStack(alignment: .leading, spacing: 0) { items(separated: false) }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityIdentifier("profile-follow-counts")
    }

    @ViewBuilder private func items(separated: Bool) -> some View {
        link(
            FollowCountFormatter.count(counts.followers), FollowCountFormatter.followersNoun(counts.followers),
            .followers, id: "profile-followers")
        if separated { dot }
        link(FollowCountFormatter.count(counts.following), "following", .following, id: "profile-following")
        if separated { dot }
        item(FollowCountFormatter.count(counts.cabals), counts.cabals == 1 ? "cabal" : "cabals")
            .accessibilityIdentifier("profile-stat-cabals")
    }

    private var dot: some View {
        Text(" \u{00B7} ")
            .font(MonacoTheme.Typo.subhead)
            .foregroundStyle(MonacoTheme.secondaryText)
            .accessibilityHidden(true)
    }

    private func link(_ count: String, _ noun: String, _ kind: FollowListKind, id: String) -> some View {
        NavigationLink(value: AnyAppRoute(FollowListRoute(userID: userID, kind: kind))) {
            item(count, noun)
        }
        .buttonStyle(.plain)
        .accessibilityIdentifier(id)
    }

    private func item(_ count: String, _ noun: String) -> some View {
        HStack(spacing: MonacoTheme.Space.xs) {
            Text(count)
                .font(MonacoTheme.Typo.subheadStrong)
                .foregroundStyle(MonacoTheme.primaryText)
            Text(noun)
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.secondaryText)
        }
        .fixedSize(horizontal: false, vertical: true)
        .frame(minHeight: MonacoButtonMetrics.hitTarget)
        .contentShape(Rectangle())
        .accessibilityElement(children: .combine)
    }
}

private struct ProfileCopyText: View {
    let line: ProfileLine

    var body: some View {
        line.runs.reduce(Text("")) { text, run in
            text
                + Text(run.text)
                .font(run.isStrong ? MonacoTheme.Typo.subheadStrong : MonacoTheme.Typo.subhead)
                .foregroundStyle(run.isStrong ? MonacoTheme.primaryText : MonacoTheme.secondaryText)
        }
        .multilineTextAlignment(.leading)
        .fixedSize(horizontal: false, vertical: true)
    }
}
