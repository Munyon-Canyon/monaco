import SwiftUI

nonisolated enum FollowListKind: Hashable, Sendable {
    case followers
    case following
}

nonisolated struct FollowListRoute: AppRoute {
    let userID: String
    let kind: FollowListKind

    @MainActor func destination() -> FollowListView {
        FollowListView(userID: userID, kind: kind)
    }
}

struct FollowListView: View {
    let userID: String
    let kind: FollowListKind

    private var title: String {
        switch kind {
        case .followers: "Followers"
        case .following: "Following"
        }
    }

    private var caption: String {
        switch kind {
        case .followers: "Followers show up here soon."
        case .following: "People you follow show up here soon."
        }
    }

    var body: some View {
        ScrollView {
            Text(caption)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(MonacoTheme.Space.m)
                .accessibilityIdentifier("follow-list-coming")
        }
        .monacoCanvas()
        .navigationTitle(title)
        .navigationBarTitleDisplayMode(.inline)
    }
}
