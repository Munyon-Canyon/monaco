import MonacoAPI
import MonacoCore
import SwiftUI

struct PeopleSearchResults: View {
    let model: PeopleSearchModel

    @Environment(AppEnvironment.self) private var environment

    var body: some View {
        switch model.state {
        case .idle, .loading:
            BoardRowSkeleton()
                .accessibilityIdentifier("friends-search-loading")
        case .loaded(let users) where users.isEmpty:
            EmptyState(title: "No one called \u{201C}\(model.query)\u{201D}")
                .accessibilityIdentifier("friends-search-empty")
        case .loaded(let users):
            MonacoGroupedList {
                ForEach(Array(users.enumerated()), id: \.element.userId) { index, user in
                    Button {
                        open(user)
                    } label: {
                        PeopleSearchRow(user: user, isLast: index == users.count - 1)
                    }
                    .buttonStyle(.monacoRow)
                    .accessibilityIdentifier("friends-search-\(user.handle)")
                }
            }
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier("friends-search-results")
        case .failed:
            MonacoErrorRow(thing: "people", identifier: "friends-search-error", retry: model.retry)
        }
    }

    private func open(_ user: Components.Schemas.UserSummary) {
        let preview = UserPreview(displayName: user.displayName, handle: user.handle, photoURL: user.photoUrl)
        environment.navigator.open(UserProfileRoute(userID: user.userId, preview: preview), in: .profile)
    }
}

private struct PeopleSearchRow: View {
    let user: Components.Schemas.UserSummary
    let isLast: Bool

    private var name: String { user.displayName.isEmpty ? user.handle : user.displayName }

    var body: some View {
        MonacoRow(
            title: name,
            subtitle: "@\(user.handle)",
            chevron: true,
            isLast: isLast,
            leading: { MonacoAvatar(photoURL: user.photoUrl, displayName: name, seed: user.userId) }
        )
    }
}
