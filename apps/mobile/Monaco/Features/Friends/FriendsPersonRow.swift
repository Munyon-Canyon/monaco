import MonacoAPI
import MonacoCore
import SwiftUI

struct FriendsPerson: Identifiable, Equatable {
    let id: String
    let handle: String
    let displayName: String
    let photoURL: String?
    var followedByMe: Bool

    var name: String { displayName.isEmpty ? handle : displayName }

    var preview: UserPreview {
        UserPreview(displayName: name, handle: handle, photoURL: photoURL)
    }

    func matches(_ query: String) -> Bool {
        let needle = query.lowercased()
        return needle.isEmpty || name.lowercased().contains(needle) || handle.lowercased().contains(needle)
    }
}

extension FriendsPerson {
    init(_ user: Components.Schemas.UserSummary) {
        self.init(
            id: user.userId, handle: user.handle, displayName: user.displayName, photoURL: user.photoUrl,
            followedByMe: user.followedByMe)
    }

    init(_ friend: FriendOnMonaco) {
        self.init(
            id: friend.id, handle: friend.handle, displayName: friend.displayName, photoURL: friend.photoURL,
            followedByMe: friend.followedByMe)
    }

    init(_ user: FollowListUser) {
        self.init(
            id: user.id, handle: user.handle, displayName: user.displayName, photoURL: user.photoURL,
            followedByMe: user.followedByMe)
    }
}

struct FriendsPersonRow: View {
    enum Accessory {
        case follow(isBusy: Bool, identifier: String, action: () -> Void)
        case badge
        case plain
    }

    let person: FriendsPerson
    var accessory: Accessory = .plain
    var isLast = false

    private var isInteractive: Bool {
        if case .follow = accessory { return true }
        return false
    }

    var body: some View {
        NavigationLink(value: AnyAppRoute(UserProfileRoute(userID: person.id, preview: person.preview))) {
            MonacoRow(
                title: person.name,
                subtitle: "@\(person.handle)",
                chevron: true,
                isLast: isLast,
                trailingIsInteractive: isInteractive,
                leading: { MonacoAvatar(photoURL: person.photoURL, displayName: person.name, seed: person.id) },
                trailing: { trailing }
            )
        }
        .buttonStyle(.monacoRow)
    }

    @ViewBuilder private var trailing: some View {
        switch accessory {
        case .follow(let isBusy, let identifier, let action):
            FollowToggle(isFollowing: person.followedByMe, isBusy: isBusy, action: action, identifier: identifier)
        case .badge:
            StatusBadge("Following", tone: .neutral)
        case .plain:
            EmptyView()
        }
    }
}

enum FriendsTab: String, CaseIterable, Hashable {
    case discover
    case following

    var title: String {
        switch self {
        case .discover: "Discover"
        case .following: "Following"
        }
    }
}

struct FriendsPeopleFrame<Content: View>: View {
    @Binding var query: String
    @Binding var tab: FriendsTab
    let showsTabs: Bool
    @ViewBuilder let content: Content

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            MonacoSearchField(placeholder: "Search people by name or handle", text: $query)
                .textInputAutocapitalization(.never)
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .accessibilityIdentifier("friends-search-field")
            if showsTabs {
                MonacoSegmented(
                    FriendsTab.allCases, selection: $tab, identifier: { "friends-tab-\($0.rawValue)" }
                ) { $0.title }
                .padding(.horizontal, MonacoTheme.Space.gutter)
            }
            content
        }
    }
}

struct FriendsSearchNotice: View {
    static let message = "Search is explicit. Monaco never follows anyone for you."

    var body: some View {
        MonacoNotice(.info, message: Self.message)
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .accessibilityIdentifier("friends-notice")
    }
}

struct FriendsNoResults: View {
    let clear: () -> Void

    var body: some View {
        EmptyState(
            title: "No people found",
            message: "Try another name or handle.",
            actionTitle: "Clear search",
            actionIdentifier: "friends-search-clear",
            systemImage: MonacoIcon.search.symbol,
            action: clear
        )
    }
}

struct FriendsFollowingEmpty: View {
    var body: some View {
        EmptyState(
            title: "You're not following anyone yet",
            message: "Search for people you know.",
            systemImage: "person.2"
        )
    }
}

struct FriendsFollowingList: View {
    let model: UserProfileModel
    let filter: String
    let clearSearch: () -> Void

    var body: some View {
        content
            .task {
                if model.followingPhase == .loading && model.followingRows.isEmpty {
                    await model.loadFollowing()
                } else {
                    await model.refreshFollowing()
                }
            }
    }

    @ViewBuilder private var content: some View {
        switch model.followingPhase {
        case .loading:
            MonacoRowSkeleton(rows: 4, markShape: .circle)
                .accessibilityElement(children: .ignore)
                .accessibilityLabel("Loading people you follow")
                .accessibilityIdentifier("friends-following-loading")
        case .empty:
            FriendsFollowingEmpty()
                .accessibilityIdentifier("friends-following-empty")
        case .failed:
            EmptyState.loadFailed(identifier: "friends-following-error") {
                Task { await model.loadFollowing() }
            }
        case .loaded:
            list
        }
    }

    @ViewBuilder private var list: some View {
        let rows = model.followingRows.map { FriendsPerson($0) }.filter { $0.matches(filter) }
        let lastID = rows.last?.id
        let loadingMore = model.followingLoadingMore
        let pageFailed = model.followingPageFailed
        if rows.isEmpty {
            FriendsNoResults(clear: clearSearch)
                .accessibilityIdentifier("friends-following-no-results")
        } else {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                MonacoGroupedList {
                    LazyVStack(spacing: 0) {
                        ForEach(rows) { person in
                            FriendsPersonRow(
                                person: person, accessory: .badge,
                                isLast: person.id == lastID && !loadingMore && !pageFailed
                            )
                            .accessibilityIdentifier("friends-following-row-\(person.id)")
                            .onAppear {
                                guard person.id == lastID else { return }
                                Task { await model.loadMoreFollowing() }
                            }
                        }
                        if pageFailed {
                            MonacoErrorRow(thing: "more people", identifier: "friends-following-page-error") {
                                Task { await model.retryFollowing() }
                            }
                        }
                    }
                    if loadingMore {
                        MonacoRowSkeleton(rows: 1, markShape: .circle)
                    }
                }
                FriendsSearchNotice()
            }
        }
    }
}
