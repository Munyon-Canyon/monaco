import MonacoCore
import SwiftUI

struct FriendsOnMonacoView: View {
    @Bindable var model: FriendsOnMonacoModel

    var body: some View {
        switch model.phase {
        case .idle, .checking:
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                Text("Checking your contacts…")
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                MonacoRowSkeleton(markShape: .circle)
            }
            .accessibilityIdentifier("friends-checking")
        case .failed:
            MonacoErrorRow(thing: "your friends", identifier: "friends-try-again") {
                Task { await model.retry() }
            }
        case .empty:
            EmptyState(title: "None of your contacts are on Monaco yet")
                .accessibilityIdentifier("friends-empty")
        case .loaded:
            MonacoGroupedList {
                ForEach(Array(model.friends.enumerated()), id: \.element.id) { index, friend in
                    FriendMatchRow(
                        friend: friend,
                        isLast: index == model.friends.count - 1,
                        isToggling: model.isToggling(friend.id)
                    ) {
                        Task { await model.follow(friend.id) }
                    }
                }
            }
        }
    }
}

private struct FriendMatchRow: View {
    let friend: FriendOnMonaco
    let isLast: Bool
    let isToggling: Bool
    let follow: () -> Void

    private var name: String { friend.displayName.isEmpty ? friend.handle : friend.displayName }

    var body: some View {
        NavigationLink(
            value: AnyAppRoute(
                UserProfileRoute(
                    userID: friend.id,
                    preview: UserPreview(displayName: name, handle: friend.handle, photoURL: friend.photoURL)
                ))
        ) {
            MonacoRow(title: name, subtitle: "@\(friend.handle)", isLast: isLast, trailingIsInteractive: true) {
                MonacoAvatar(photoURL: friend.photoURL, displayName: name, seed: friend.id)
            } trailing: {
                FollowToggle(
                    isFollowing: friend.followedByMe, isBusy: isToggling,
                    action: friend.followedByMe ? nil : follow,
                    identifier: "friends-follow-\(friend.handle)")
            }
        }
        .buttonStyle(.monacoRow)
        .accessibilityIdentifier("friends-row-\(friend.handle)")
    }
}
