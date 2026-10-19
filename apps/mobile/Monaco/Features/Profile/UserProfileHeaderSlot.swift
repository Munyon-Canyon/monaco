import MonacoCore
import SwiftUI

enum UserProfileHeaderSlot: UserProfileSection {
    static let isLive = true

    static func body(for context: UserProfileContext) -> some View {
        UserProfileHeader(context: context)
    }
}

struct UserProfileFixture: Equatable {
    var bio: String?
    var performance = IdentityPerformance.unavailable
    var periodText = "1M"
    var joinedText: String?
    var showsPublicActivity = false
}

extension EnvironmentValues {
    @Entry var userProfileFixture = UserProfileFixture()
}

enum UserProfileCopy {
    static let ownNotice = "This is how your profile appears to others. Your account balances stay private."
    static let blockedNotice = "You blocked this member. They can't follow you, and you won't see their messages."
    static let privateTitle = "Performance is private"
    static let privateMessage = "Following does not reveal private balances or conversations."
    static let notFoundTitle = "Profile not found"
    static let notFoundMessage = "This profile isn't available."
}

struct UserProfileNotFound: View {
    let findPeople: () -> Void

    var body: some View {
        EmptyState(
            title: UserProfileCopy.notFoundTitle, message: UserProfileCopy.notFoundMessage,
            actionTitle: "Find people", actionIdentifier: "user-profile-find-people", systemImage: "person",
            action: findPeople
        )
        .accessibilityIdentifier("user-profile-unavailable")
    }
}

struct UserProfileLoadFailed: View {
    let retry: () -> Void

    var body: some View {
        EmptyState(
            title: EditProfileCopy.failedTitle, message: EditProfileCopy.failedMessage,
            actionTitle: "Try again", actionIdentifier: "user-profile-error", systemImage: MonacoIcon.offline.symbol,
            action: retry
        )
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
        VStack(spacing: MonacoTheme.Space.m) {
            content
        }
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

    @ViewBuilder private var content: some View {
        switch model?.phase ?? .loading {
        case .idle, .loading:
            loading
        case .unavailable:
            UserProfileNotFound {
                environment.navigator.open(FriendsRoute(), in: environment.navigator.selectedTab)
            }
        case .failed:
            UserProfileLoadFailed { Task { await model?.load() } }
        case .loaded:
            UserProfileLoaded(userID: userID, isViewer: isViewer)
        }
    }

    @ViewBuilder private var loading: some View {
        if let preview = context.preview {
            IdentityCard(
                model: IdentityCardModel(
                    displayName: preview.displayName, handle: preview.handle, performance: .loading,
                    periodText: ""),
                identifierPrefix: "user-profile"
            ) {
                MonacoAvatar(
                    photoURL: preview.photoURL, displayName: preview.displayName, size: 64, seed: userID,
                    ring: .identity)
            } share: {
                EmptyView()
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .accessibilityIdentifier("user-profile-loading")
        } else {
            IdentityCardSkeleton(identifier: "user-profile-loading", label: "Loading profile")
                .padding(.horizontal, MonacoTheme.Space.gutter)
        }
    }
}

struct UserProfileContentData: Equatable {
    var userID: String
    var title: String
    var handle: String?
    var photoURL: String?
    var followers: Int
    var following: Int
    var isViewer = false
    var followedByMe = false
    var blockedByMe = false
    var isBusy = false
}

struct UserProfileActions {
    var follow: () -> Void = {}
    var unfollow: () -> Void = {}
    var unblock: () -> Void = {}
    var openPublicActivity: () -> Void = {}
}

private struct UserProfileLoaded: View {
    let userID: String
    let isViewer: Bool

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(UserProfileModel.self) private var model: UserProfileModel?
    @Environment(\.userProfileFixture) private var fixture

    var body: some View {
        UserProfileContent(data: data, fixture: fixture, actions: actions)
    }

    private var data: UserProfileContentData {
        let name = model?.displayName ?? ""
        let handle = model?.handle ?? ""
        return UserProfileContentData(
            userID: userID, title: name.isEmpty ? (handle.isEmpty ? "Member" : handle) : name,
            handle: handle.isEmpty ? nil : handle, photoURL: model?.photoURL,
            followers: model?.followerCount ?? 0, following: model?.followingCount ?? 0, isViewer: isViewer,
            followedByMe: model?.followedByMe ?? false, blockedByMe: model?.blockedByMe ?? false,
            isBusy: model?.isToggling(userID) ?? false)
    }

    private var actions: UserProfileActions {
        UserProfileActions(
            follow: { Task { await model?.follow(userID: userID) } },
            unfollow: { Task { await model?.unfollow(userID: userID) } },
            unblock: {
                guard let model else { return }
                Task {
                    let flow = Flow20aReportBlockModel(api: environment.api, hints: environment.hints)
                    await UserProfileBlockRunner.run(false, flow: flow, model: model, toasts: toasts)
                }
            },
            openPublicActivity: { environment.navigator.selectedTab = .feed })
    }
}

struct UserProfileContent: View {
    let data: UserProfileContentData
    let fixture: UserProfileFixture
    let actions: UserProfileActions

    var body: some View {
        VStack(spacing: MonacoTheme.Space.m) {
            IdentityCard(model: cardModel, identifierPrefix: "user-profile") {
                MonacoAvatar(
                    photoURL: data.photoURL, displayName: data.title, size: 64, seed: data.userID, ring: .identity)
            } share: {
                EmptyView()
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            SocialCounts(
                model: SocialCountsModel(
                    userID: data.userID, followers: data.followers, following: data.following, cabals: nil),
                identifierPrefix: "user-profile"
            )
            action
            extras
        }
    }

    private var cardModel: IdentityCardModel {
        IdentityCardModel(
            displayName: data.title, handle: data.handle == data.title ? nil : data.handle,
            joinedText: fixture.joinedText, bio: fixture.bio, positionsValue: nil, performance: fixture.performance,
            periodText: fixture.periodText)
    }

    @ViewBuilder private var action: some View {
        if data.isViewer {
            MonacoNotice(.info, message: UserProfileCopy.ownNotice)
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .accessibilityIdentifier("user-profile-own-notice")
        } else if data.blockedByMe {
            VStack(spacing: MonacoTheme.Space.sm) {
                MonacoNotice(.info, message: UserProfileCopy.blockedNotice)
                Button("Unblock", action: actions.unblock)
                    .buttonStyle(.monacoOutline)
                    .accessibilityIdentifier("user-profile-unblock-button")
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
        } else if data.followedByMe {
            Button("Following", action: actions.unfollow)
                .buttonStyle(.monacoOutline)
                .disabled(data.isBusy)
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .accessibilityIdentifier("user-profile-follow")
        } else {
            Button("Follow", action: actions.follow)
                .buttonStyle(.monacoPrimary)
                .disabled(data.isBusy)
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .accessibilityIdentifier("user-profile-follow")
        }
    }

    @ViewBuilder private var extras: some View {
        if !data.blockedByMe {
            if fixture.performance == .hidden {
                EmptyState(
                    title: UserProfileCopy.privateTitle, message: UserProfileCopy.privateMessage,
                    systemImage: MonacoIcon.lock.symbol
                )
                .accessibilityIdentifier("user-profile-private")
            } else if fixture.showsPublicActivity {
                Button(action: actions.openPublicActivity) {
                    MonacoRow(
                        title: "Public activity", subtitle: "Latest proposals and discussions", chevron: true,
                        isLast: true, leading: { SunkenGlyphMark(icon: .news) })
                }
                .buttonStyle(.monacoRow)
                .accessibilityIdentifier("user-profile-public-activity")
            }
        }
    }
}
