import MonacoCore
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

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @State private var model: UserProfileModel?

    private var title: String {
        switch kind {
        case .followers: "Followers"
        case .following: "Following"
        }
    }

    private var emptyTitle: String {
        switch kind {
        case .followers: "No followers yet"
        case .following: "Not following anyone yet"
        }
    }

    var body: some View {
        ScrollView { content }
            .refreshable { await reload() }
            .monacoCanvas()
            .navigationTitle(title)
            .navigationBarTitleDisplayMode(.inline)
            .task(id: userID) {
                let model = prepared()
                guard needsFirstLoad(model) else { return }
                await reload()
            }
            .onChange(of: model?.toastTick) {
                if let error = model?.lastError { toasts.show(error) }
            }
    }

    @ViewBuilder private var content: some View {
        switch phase {
        case .loading:
            BoardRowSkeleton(rows: 3)
                .accessibilityElement(children: .ignore)
                .accessibilityLabel("Loading \(title)")
                .accessibilityIdentifier("follow-list-loading")
        case .empty:
            EmptyState(title: emptyTitle)
                .accessibilityIdentifier("follow-list-empty")
        case .failed:
            MonacoErrorRow(thing: "this list", identifier: "follow-list-error") {
                Task { await reload() }
            }
        case .loaded:
            list
        }
    }

    private var list: some View {
        let rows = people
        let loadingMore = isLoadingMore
        return MonacoGroupedList(rules: loadingMore ? .top : .both) {
            LazyVStack(spacing: 0) {
                ForEach(rows) { user in
                    FollowListPersonRow(
                        user: user,
                        isViewer: user.id == environment.viewer?.userID,
                        isLast: user.id == rows.last?.id && !loadingMore && !canRetryPage,
                        isToggling: model?.isToggling(user.id) == true
                    ) {
                        Task { await toggle(user) }
                    }
                    .onAppear {
                        guard user.id == rows.last?.id else { return }
                        Task { await loadMore() }
                    }
                }
                if canRetryPage {
                    Button("Try again") { Task { await retry() } }
                        .buttonStyle(.monacoSecondary)
                        .frame(maxWidth: .infinity, minHeight: 44)
                        .padding(.horizontal, MonacoTheme.Space.m)
                        .accessibilityIdentifier("follow-list-page-retry")
                }
            }
            if loadingMore {
                BoardRowSkeleton(rows: 1)
            }
        }
    }

    private var phase: UserProfileModel.ListPhase {
        guard let model else { return .loading }
        switch kind {
        case .followers: return model.followersPhase
        case .following: return model.followingPhase
        }
    }

    private var people: [FollowListUser] {
        guard let model else { return [] }
        switch kind {
        case .followers: return model.followerRows
        case .following: return model.followingRows
        }
    }

    private var isLoadingMore: Bool {
        switch kind {
        case .followers: model?.followersLoadingMore == true
        case .following: model?.followingLoadingMore == true
        }
    }

    private var canRetryPage: Bool {
        switch kind {
        case .followers: model?.followersPageFailed == true
        case .following: model?.followingPageFailed == true
        }
    }

    private func needsFirstLoad(_ model: UserProfileModel) -> Bool {
        switch kind {
        case .followers: model.followersPhase == .loading && model.followerRows.isEmpty
        case .following: model.followingPhase == .loading && model.followingRows.isEmpty
        }
    }

    private func prepared() -> UserProfileModel {
        if let model, model.userID == userID { return model }
        let created = UserProfileModel(userID: userID, api: environment.api)
        model = created
        return created
    }

    private func reload() async {
        guard let model else { return }
        switch kind {
        case .followers: await model.loadFollowers()
        case .following: await model.loadFollowing()
        }
    }

    private func retry() async {
        guard let model else { return }
        switch kind {
        case .followers: await model.retryFollowers()
        case .following: await model.retryFollowing()
        }
    }

    private func loadMore() async {
        guard let model else { return }
        switch kind {
        case .followers: await model.loadMoreFollowers()
        case .following: await model.loadMoreFollowing()
        }
    }

    private func toggle(_ user: FollowListUser) async {
        guard let model else { return }
        if user.followedByMe {
            await model.unfollow(userID: user.id)
        } else {
            await model.follow(userID: user.id)
        }
    }
}

private struct FollowListPersonRow: View {
    let user: FollowListUser
    let isViewer: Bool
    let isLast: Bool
    let isToggling: Bool
    let toggle: () -> Void

    private var name: String { user.displayName.isEmpty ? user.handle : user.displayName }

    private var preview: UserPreview {
        UserPreview(displayName: user.displayName, handle: user.handle, photoURL: user.photoURL)
    }

    var body: some View {
        NavigationLink(value: AnyAppRoute(UserProfileRoute(userID: user.id, preview: preview))) {
            MonacoRow(title: name, subtitle: "@\(user.handle)", isLast: isLast, trailingIsInteractive: true) {
                MonacoAvatar(photoURL: user.photoURL, displayName: name, seed: user.id)
            } trailing: {
                if !isViewer {
                    FollowToggle(
                        isFollowing: user.followedByMe, isBusy: isToggling, action: toggle,
                        identifier: "follow-list-follow-\(user.id)")
                }
            }
        }
        .buttonStyle(.monacoRow)
        .accessibilityIdentifier("follow-list-open-\(user.id)")
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("follow-list-row-\(user.id)")
    }
}
