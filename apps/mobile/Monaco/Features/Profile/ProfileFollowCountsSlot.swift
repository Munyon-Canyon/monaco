import SwiftUI

enum ProfileFollowCountsSlot: ProfileSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        ProfileFollowLinks()
    }
}

private struct ProfileFollowLinks: View {
    @Environment(AppEnvironment.self) private var environment

    var body: some View {
        if let viewer = environment.viewer {
            HStack(spacing: 0) {
                link("Followers", FollowListRoute(userID: viewer.userID, kind: .followers), id: "profile-followers")
                Text(" · ")
                    .font(MonacoTheme.Typo.calloutStrong)
                    .foregroundStyle(MonacoTheme.muted)
                    .accessibilityHidden(true)
                link("Following", FollowListRoute(userID: viewer.userID, kind: .following), id: "profile-following")
            }
            .frame(maxWidth: .infinity)
        }
    }

    private func link(_ title: String, _ route: FollowListRoute, id: String) -> some View {
        NavigationLink(value: AnyAppRoute(route)) {
            Text(title)
                .font(MonacoTheme.Typo.calloutStrong)
                .foregroundStyle(MonacoTheme.ink)
                .frame(minHeight: 44)
        }
        .buttonStyle(.plain)
        .accessibilityIdentifier(id)
    }
}
