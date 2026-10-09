import MonacoCore
import SwiftUI

enum UserProfileHeaderSlot: UserProfileSection {
    static let isLive = true

    static func body(for context: UserProfileContext) -> some View {
        UserProfileHeader(context: context)
    }
}

private struct UserProfileHeader: View {
    let context: UserProfileContext
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(UserProfileModel.self) private var model: UserProfileModel?

    private var userID: String { context.userID }
    private var isViewer: Bool { userID == environment.viewer?.userID }

    var body: some View {
        Group {
            switch model?.phase ?? .loading {
            case .idle, .loading:
                if let preview = context.preview {
                    previewIdentity(preview)
                } else {
                    skeleton
                }
            case .unavailable:
                EmptyState(title: "This account isn't available.")
                    .accessibilityIdentifier("user-profile-unavailable")
            case .failed:
                loadFailure
            case .loaded:
                identity
            }
        }
        .padding(.top, MonacoTheme.Space.m)
        .frame(maxWidth: .infinity)
        .toolbar {
            if model?.phase == .loaded, !isViewer {
                ToolbarItem(placement: .topBarTrailing) { UserProfileMoreMenu() }
            }
        }
        .onChange(of: model?.toastTick) {
            if let error = model?.lastError { toasts.show(error) }
        }
    }

    private var skeleton: some View {
        VStack(spacing: MonacoTheme.Space.s) {
            SkeletonBlock(width: 96, height: 96, radius: 48)
            SkeletonBlock(width: 168, height: 28)
            SkeletonBlock(width: 200, height: 14)
            SkeletonBlock(
                width: 160, height: MonacoButtonMetrics.minimumHeight, radius: MonacoButtonMetrics.minimumHeight / 2)
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("Loading profile")
        .accessibilityIdentifier("user-profile-loading")
    }

    private var loadFailure: some View {
        MonacoErrorRow(thing: "this profile", identifier: "user-profile-error") {
            Task { await model?.load() }
        }
    }

    private func previewIdentity(_ preview: UserPreview) -> some View {
        VStack(spacing: MonacoTheme.Space.s) {
            MonacoAvatar(photoURL: preview.photoURL, displayName: preview.displayName, size: 96, seed: userID)
            Text(preview.displayName)
                .font(MonacoTheme.Typo.display)
                .foregroundStyle(MonacoTheme.ink)
                .multilineTextAlignment(.center)
                .accessibilityIdentifier("user-profile-name")
            SkeletonBlock(width: 200, height: 14)
            SkeletonBlock(
                width: 160, height: MonacoButtonMetrics.minimumHeight, radius: MonacoButtonMetrics.minimumHeight / 2)
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("user-profile-loading")
    }

    private var identity: some View {
        VStack(spacing: MonacoTheme.Space.s) {
            MonacoAvatar(
                photoURL: model?.photoURL, displayName: title, size: 96, seed: userID)
            Text(title)
                .font(MonacoTheme.Typo.display)
                .foregroundStyle(MonacoTheme.ink)
                .multilineTextAlignment(.center)
                .accessibilityIdentifier("user-profile-name")
            if let handle, handle != title {
                Text(handle)
                    .font(MonacoTheme.Typo.bodyStrong)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .accessibilityIdentifier("user-profile-handle")
            }
            counts
            if !isViewer, let model {
                followButton(model)
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("user-profile-header")
    }

    private var title: String {
        let name = model?.displayName ?? ""
        if !name.isEmpty { return name }
        return handle ?? "Member"
    }

    private var handle: String? {
        guard let value = model?.handle, !value.isEmpty else { return nil }
        return "@\(value)"
    }

    private var counts: some View {
        HStack(spacing: 0) {
            link(FollowCountFormatter.followers(model?.followerCount ?? 0), .followers, id: "user-profile-followers")
            Text(" · ")
                .font(MonacoTheme.Typo.calloutStrong)
                .foregroundStyle(MonacoTheme.muted)
                .accessibilityHidden(true)
            link(FollowCountFormatter.following(model?.followingCount ?? 0), .following, id: "user-profile-following")
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

    @ViewBuilder
    private func followButton(_ model: UserProfileModel) -> some View {
        let following = model.followedByMe
        Group {
            if following {
                Button("Following") { Task { await model.unfollow(userID: model.userID) } }
                    .buttonStyle(.monacoSecondary)
                    .accessibilityIdentifier("user-profile-follow")
            } else {
                Button("Follow") { Task { await model.follow(userID: model.userID) } }
                    .buttonStyle(.monacoPrimary)
                    .accessibilityIdentifier("user-profile-follow")
            }
        }
        .disabled(model.isToggling(model.userID))
    }
}
