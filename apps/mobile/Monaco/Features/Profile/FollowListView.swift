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
            EmptyState(title: "Couldn't load this list.", actionTitle: "Try again") {
                Task { await reload() }
            }
            .accessibilityIdentifier("follow-list-error")
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
                    Button("Try again") { Task { await loadMore() } }
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
        ViewThatFits(in: .horizontal) {
            row(stacked: false)
            row(stacked: true)
        }
        .accessibilityIdentifier("follow-list-row-\(user.id)")
    }

    private func row(stacked: Bool) -> some View {
        let identity = NavigationLink(value: AnyAppRoute(UserProfileRoute(userID: user.id, preview: preview))) {
            HStack(spacing: MonacoTheme.Space.sm) {
                MonacoAvatar(photoURL: user.photoURL, displayName: name, size: 40, seed: user.id)
                VStack(alignment: .leading, spacing: 2) {
                    Text(name)
                        .font(MonacoTheme.Typo.rowTitle)
                        .foregroundStyle(MonacoTheme.ink)
                        .lineLimit(stacked ? 2 : 1)
                    Text("@\(user.handle)")
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.muted)
                        .lineLimit(1)
                }
                .frame(maxWidth: .infinity, alignment: .leading)
            }
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)

        return Group {
            if stacked {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                    identity
                    if !isViewer { follow }
                }
            } else {
                HStack(spacing: MonacoTheme.Space.s) {
                    identity
                    if !isViewer { follow }
                }
            }
        }
        .padding(.horizontal, MonacoTheme.Space.m)
        .padding(.vertical, 8)
        .frame(minHeight: 60, alignment: .leading)
        .overlay(alignment: .bottom) {
            if !isLast {
                Rectangle()
                    .fill(MonacoTheme.hairline)
                    .frame(height: 1)
                    .padding(.leading, MonacoTheme.Space.m + 40 + MonacoTheme.Space.sm)
            }
        }
    }

    @ViewBuilder private var follow: some View {
        if user.followedByMe {
            Button("Following", action: toggle)
                .buttonStyle(.monacoSecondary)
                .disabled(isToggling)
        } else {
            Button("Follow", action: toggle)
                .buttonStyle(.monacoPrimary)
                .disabled(isToggling)
        }
    }
}
