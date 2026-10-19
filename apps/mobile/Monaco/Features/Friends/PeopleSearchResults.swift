import MonacoAPI
import MonacoCore
import SwiftUI

struct PeopleSearchResults: View {
    @Bindable var model: PeopleSearchModel

    @Environment(AppEnvironment.self) private var environment

    var body: some View {
        VStack(spacing: 0) { results }
            .onAppear { model.refresh() }
    }

    @ViewBuilder private var results: some View {
        switch model.state {
        case .idle, .loading:
            MonacoRowSkeleton(rows: 4, markShape: .circle)
                .accessibilityIdentifier("friends-search-loading")
        case .loaded(let users) where users.isEmpty:
            FriendsNoResults { model.query = "" }
                .accessibilityIdentifier("friends-search-empty")
        case .loaded(let users):
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                MonacoGroupedList {
                    ForEach(Array(users.enumerated()), id: \.element.userId) { index, user in
                        PeopleSearchRow(
                            user: user,
                            isViewer: user.userId == environment.viewer?.userID,
                            isLast: index == users.count - 1,
                            isToggling: model.isToggling(user.userId)
                        ) {
                            Task { await model.toggleFollow(user.userId) }
                        }
                    }
                }
                .accessibilityElement(children: .contain)
                .accessibilityIdentifier("friends-search-results")
                FriendsSearchNotice()
            }
        case .failed:
            EmptyState.loadFailed(identifier: "friends-search-error", retry: model.retry)
        }
    }
}

private struct PeopleSearchRow: View {
    let user: Components.Schemas.UserSummary
    let isViewer: Bool
    let isLast: Bool
    let isToggling: Bool
    let toggle: () -> Void

    var body: some View {
        FriendsPersonRow(
            person: FriendsPerson(user),
            accessory: isViewer
                ? .plain
                : .follow(
                    isBusy: isToggling, identifier: "friends-search-follow-\(user.userId)", action: toggle),
            isLast: isLast
        )
        .accessibilityIdentifier("friends-search-\(user.handle)")
    }
}
