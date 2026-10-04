import MonacoCore
import SwiftUI

enum UserProfileHeaderSlot: UserProfileSection {
    static let isLive = true

    static func body(for context: UserProfileContext) -> some View {
        UserProfileHeader(userID: context.userID, preview: context.preview)
    }
}

private struct UserProfileHeader: View {
    let userID: String
    let preview: UserPreview?
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @State private var model: FollowButtonModel?

    private var isViewer: Bool { userID == environment.viewer?.userID }

    var body: some View {
        Group {
            if model?.unavailable == true {
                EmptyState(title: "This account isn't available.")
                    .accessibilityIdentifier("user-profile-unavailable")
            } else {
                identity
            }
        }
        .padding(.top, MonacoTheme.Space.m)
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .frame(maxWidth: .infinity)
        .toolbar {
            if !isViewer {
                ToolbarItem(placement: .topBarTrailing) { UserProfileMoreMenu() }
            }
        }
        .task {
            if model == nil { model = FollowButtonModel(userID: userID, api: environment.api) }
        }
        .onChange(of: model?.failureTick) {
            if let error = model?.lastError { toasts.show(error) }
        }
    }

    private var identity: some View {
        VStack(spacing: MonacoTheme.Space.s) {
            MonacoAvatar(photoURL: preview?.photoURL, displayName: preview?.displayName ?? "", size: 96, seed: userID)
            name
            counts
            if !isViewer, let model {
                followButton(model)
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("user-profile-header")
    }

    @ViewBuilder
    private var name: some View {
        if let preview {
            let handle = preview.handle.map { "@\($0)" }
            let title = preview.displayName.isEmpty ? handle ?? "Member" : preview.displayName
            Text(title)
                .font(MonacoTheme.Typo.title)
                .foregroundStyle(MonacoTheme.ink)
                .multilineTextAlignment(.center)
                .accessibilityIdentifier("user-profile-name")
            if let handle, handle != title {
                Text(handle)
                    .font(MonacoTheme.Typo.bodyStrong)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .accessibilityIdentifier("user-profile-handle")
            }
        } else {
            Text("Their name shows up here soon.")
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
                .accessibilityIdentifier("user-profile-name-coming")
        }
    }

    private var counts: some View {
        HStack(spacing: 0) {
            link("Followers", .followers, id: "user-profile-followers")
            Text(" · ")
                .font(MonacoTheme.Typo.calloutStrong)
                .foregroundStyle(MonacoTheme.muted)
                .accessibilityHidden(true)
            link("Following", .following, id: "user-profile-following")
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
    private func followButton(_ model: FollowButtonModel) -> some View {
        Group {
            if model.following {
                Button("Following") { Task { await model.toggle() } }
                    .buttonStyle(.monacoSecondary)
            } else {
                Button("Follow") { Task { await model.toggle() } }
                    .buttonStyle(.monacoPrimary)
            }
        }
        .disabled(model.isToggling)
        .accessibilityIdentifier("user-profile-follow")
    }
}
