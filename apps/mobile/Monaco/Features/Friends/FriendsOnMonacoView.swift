import MonacoCore
import SwiftUI
import UIKit

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
        case .contactsFailed:
            ContactsReadErrorRow(canOpenSettings: model.access != .granted) {
                Task { await model.retry() }
            }
        case .empty:
            EmptyState(title: "None of your contacts are on Monaco yet")
                .accessibilityIdentifier("friends-empty")
        case .loaded:
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                MonacoSectionHeader("From your contacts")
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

private struct ContactsReadErrorRow: View {
    let canOpenSettings: Bool
    let retry: () -> Void

    var body: some View {
        HStack(spacing: MonacoTheme.Space.sm) {
            Text("Couldn't read your contacts.")
                .font(MonacoTheme.Typo.body)
                .foregroundStyle(MonacoTheme.ink)
                .fixedSize(horizontal: false, vertical: true)
                .frame(maxWidth: .infinity, alignment: .leading)
            if canOpenSettings {
                Button("Open Settings") {
                    guard let url = URL(string: UIApplication.openSettingsURLString) else { return }
                    UIApplication.shared.open(url)
                }
                .font(MonacoTheme.Typo.calloutStrong)
                .foregroundStyle(MonacoTheme.brand)
                .buttonStyle(.plain)
                .frame(minHeight: 44)
                .accessibilityIdentifier("friends-contacts-open-settings")
            }
            Button("Try again", action: retry)
                .font(MonacoTheme.Typo.calloutStrong)
                .foregroundStyle(MonacoTheme.brand)
                .buttonStyle(.plain)
                .frame(minHeight: 44)
                .accessibilityIdentifier("friends-contacts-retry")
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.vertical, 8)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("friends-contacts-error")
    }
}
