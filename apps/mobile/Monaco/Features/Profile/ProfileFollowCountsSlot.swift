import MonacoCore
import SwiftUI

enum ProfileFollowCountsSlot: ProfileSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        ProfileFollowCounts()
    }
}

private struct ProfileFollowCounts: View {
    @Environment(AppEnvironment.self) private var environment

    var body: some View {
        if let viewer = environment.viewer {
            ProfileFollowCountLinks(userID: viewer.userID)
        }
    }
}

private struct ProfileFollowCountLinks: View {
    let userID: String

    @Environment(AppEnvironment.self) private var environment
    @Environment(ScreenRefresh.self) private var refresh: ScreenRefresh?
    @Environment(ToastCenter.self) private var toasts
    @State private var model: UserProfileModel?

    var body: some View {
        content
            .frame(maxWidth: .infinity)
            .task(id: userID) {
                let model = prepared()
                refresh?.register("profile-follow-counts") { await model.refresh() }
                if model.profile == nil {
                    await model.load()
                } else {
                    await model.refresh()
                }
            }
            .onChange(of: model?.toastTick) {
                if let error = model?.lastError { toasts.show(error) }
            }
    }

    @ViewBuilder private var content: some View {
        switch model?.phase ?? .loading {
        case .idle, .loading:
            SkeletonBlock(width: 180, height: 14)
                .frame(minHeight: 44)
                .accessibilityElement(children: .ignore)
                .accessibilityLabel("Loading followers")
                .accessibilityIdentifier("profile-follow-counts-loading")
        case .failed, .unavailable:
            MonacoErrorRow(thing: "your followers", identifier: "profile-follow-counts-error") {
                Task { await model?.load() }
            }
        case .loaded:
            HStack(spacing: 0) {
                link("\(model?.followerCount ?? 0) Followers", .followers, id: "profile-followers")
                Text(" · ")
                    .font(MonacoTheme.Typo.calloutStrong)
                    .foregroundStyle(MonacoTheme.muted)
                    .accessibilityHidden(true)
                link("\(model?.followingCount ?? 0) Following", .following, id: "profile-following")
            }
        }
    }

    private func link(_ title: String, _ kind: FollowListKind, id: String) -> some View {
        NavigationLink(value: AnyAppRoute(FollowListRoute(userID: userID, kind: kind))) {
            Text(title)
                .font(MonacoTheme.Typo.calloutStrong)
                .foregroundStyle(MonacoTheme.ink)
                .frame(minHeight: 44)
        }
        .buttonStyle(.plain)
        .accessibilityIdentifier(id)
    }

    private func prepared() -> UserProfileModel {
        if let model, model.userID == userID { return model }
        let created = UserProfileModel(userID: userID, api: environment.api)
        model = created
        return created
    }
}
