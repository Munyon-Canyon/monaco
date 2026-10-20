import MonacoAPI
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
    @Environment(UserProfileModel.self) private var model: UserProfileModel?
    @Environment(ToastCenter.self) private var toasts

    var body: some View {
        content
            .frame(maxWidth: .infinity)
            .padding(.top, MonacoTheme.Space.m - SectionStackMetrics.spacing)
            .onChange(of: model?.toastTick) {
                if let error = model?.lastError { toasts.show(error) }
            }
    }

    @ViewBuilder private var content: some View {
        if let model {
            switch model.phase {
            case .idle, .loading:
                SkeletonBlock(width: 180, height: 14)
                    .frame(minHeight: 44)
                    .accessibilityElement(children: .ignore)
                    .accessibilityLabel("Loading followers")
                    .accessibilityIdentifier("profile-follow-counts-loading")
            case .failed, .unavailable:
                MonacoErrorRow(thing: "your followers", identifier: "profile-follow-counts-error") {
                    Task { await model.load() }
                }
            case .loaded:
                SocialCounts(
                    model: SocialCountsModel(
                        userID: model.userID, followers: model.followerCount, following: model.followingCount,
                        cabals: model.cabalCount))
            }
        }
    }
}
