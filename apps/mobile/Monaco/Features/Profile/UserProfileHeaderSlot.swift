import MonacoCore
import SwiftUI

enum UserProfileHeaderSlot: UserProfileSection {
    static let isLive = true

    static func body(for context: UserProfileContext) -> some View {
        UserProfileHeader(context: context)
    }
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
            UserProfileCard(
                data: UserProfileContentData(
                    userID: userID, title: preview.displayName, photoURL: preview.photoURL,
                    handleLine: preview.handle.map { "@\($0)" } ?? "", followers: 0, following: 0)
            )
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .accessibilityIdentifier("user-profile-loading")
        } else {
            SkeletonBlock(height: SkeletonMetrics.cardHeight, radius: MonacoTheme.Radius.card)
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .accessibilityElement(children: .ignore)
                .accessibilityLabel("Loading profile")
                .accessibilityIdentifier("user-profile-loading")
        }
    }
}

struct UserProfileContentData: Equatable {
    var userID: String
    var title: String
    var photoURL: String?
    var crown = false
    var handleLine = ""
    var bio: String?
    var pnl = "\u{2014}"
    var rightCalls = "\u{2014}"
    var followers: Int
    var following: Int
    var cabals: Int?
    var inCommon = 0
    var isViewer = false
    var followedByMe = false
    var blockedByMe = false
    var isBusy = false
    var performanceHidden = false
}

struct UserProfileActions {
    var follow: () -> Void = {}
    var unfollow: () -> Void = {}
    var unblock: () -> Void = {}
}

private struct UserProfileLoaded: View {
    let userID: String
    let isViewer: Bool

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(UserProfileModel.self) private var model: UserProfileModel?

    var body: some View {
        UserProfileContent(data: data, actions: actions)
    }

    private var data: UserProfileContentData {
        let name = model?.displayName ?? ""
        let handle = model?.handle ?? ""
        return UserProfileContentData(
            userID: userID, title: name.isEmpty ? (handle.isEmpty ? "Member" : handle) : name,
            photoURL: model?.photoURL, crown: model?.crown ?? false, handleLine: model?.handleLine ?? "",
            bio: model?.bio, pnl: model?.pnlFigure ?? "\u{2014}", rightCalls: model?.rightCallsFigure ?? "\u{2014}",
            followers: model?.followerCount ?? 0, following: model?.followingCount ?? 0,
            cabals: model?.cabalCount, inCommon: model?.inCommon ?? 0, isViewer: isViewer,
            followedByMe: model?.followedByMe ?? false, blockedByMe: model?.blockedByMe ?? false,
            isBusy: model?.isToggling(userID) ?? false, performanceHidden: model?.performance == .hidden)
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
            })
    }
}

struct UserProfileContent: View {
    let data: UserProfileContentData
    let actions: UserProfileActions

    var body: some View {
        VStack(spacing: MonacoTheme.Space.m) {
            UserProfileCard(data: data)
                .padding(.horizontal, MonacoTheme.Space.gutter)
            UserProfileCounts(data: data)
            action
            extras
        }
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
            Button(action: actions.follow) {
                Label("Follow", systemImage: "person.badge.plus")
            }
            .buttonStyle(.monacoPrimary)
            .disabled(data.isBusy)
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .accessibilityIdentifier("user-profile-follow")
        }
    }

    @ViewBuilder private var extras: some View {
        if !data.blockedByMe, data.performanceHidden {
            EmptyState(
                title: UserProfileCopy.privateTitle, message: UserProfileCopy.privateMessage,
                systemImage: MonacoIcon.lock.symbol
            )
            .accessibilityIdentifier("user-profile-private")
        }
    }
}

private struct UserProfileCard: View {
    let data: UserProfileContentData

    var body: some View {
        MonacoCard {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
                HStack(alignment: .center, spacing: MonacoTheme.Space.sm) {
                    photo
                    names
                }
                if let bio = data.bio {
                    Text(bio)
                        .font(MonacoTheme.Typo.body)
                        .foregroundStyle(MonacoTheme.primaryText)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .fixedSize(horizontal: false, vertical: true)
                        .accessibilityIdentifier("user-profile-bio")
                }
                MonacoRule()
                HStack(alignment: .top, spacing: MonacoTheme.Space.sm) {
                    figure("P&L \u{00B7} 1M", data.pnl, tint: Self.tint(data.pnl), id: "user-profile-pnl")
                    figure(
                        "Right calls \u{00B7} 1M", data.rightCalls, tint: MonacoTheme.primaryText,
                        id: "user-profile-right-calls")
                }
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("user-profile-card")
    }

    private var photo: some View {
        MonacoAvatar(
            photoURL: data.photoURL, displayName: data.title, size: ProfileCardMetrics.photo, seed: data.userID,
            ring: .identity
        )
        .overlay(alignment: .top) {
            if data.crown {
                Image(systemName: "crown.fill")
                    .font(.body.weight(.semibold))
                    .foregroundStyle(MonacoTheme.goldGlyph)
                    .offset(y: -10)
                    .accessibilityLabel("Month leader")
                    .accessibilityIdentifier("user-profile-crown")
            }
        }
    }

    private var names: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
            Text(data.title)
                .font(MonacoTheme.Typo.title2)
                .foregroundStyle(MonacoTheme.primaryText)
                .lineLimit(2)
                .minimumScaleFactor(0.7)
                .accessibilityIdentifier("user-profile-name")
                .monacoPrivate()
            if !data.handleLine.isEmpty {
                Text(data.handleLine)
                    .font(MonacoTheme.Typo.subhead)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .lineLimit(1)
                    .minimumScaleFactor(0.7)
                    .accessibilityIdentifier("user-profile-handle")
                    .monacoPrivate()
            }
        }
        .frame(maxWidth: .infinity, minHeight: ProfileCardMetrics.nameMinHeight, alignment: .leading)
    }

    private func figure(_ label: String, _ value: String, tint: Color, id: String) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            Text(label)
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.secondaryText)
                .fixedSize(horizontal: false, vertical: true)
            Text(value)
                .moneyFigure(.card)
                .foregroundStyle(tint)
                .lineLimit(1)
                .minimumScaleFactor(MoneyStyle.card.minimumScaleFactor)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(label)
        .accessibilityValue(value)
        .accessibilityIdentifier(id)
    }

    private static func tint(_ pnl: String) -> Color {
        if pnl.hasPrefix("+") { return MonacoTheme.profit }
        if pnl.hasPrefix("\u{2212}") { return MonacoTheme.loss }
        return MonacoTheme.primaryText
    }
}

private struct UserProfileCounts: View {
    let data: UserProfileContentData

    var body: some View {
        HStack(spacing: MonacoTheme.Space.s) {
            link(
                FollowCountFormatter.count(data.followers), FollowCountFormatter.followersNoun(data.followers),
                .followers, id: "user-profile-followers")
            link(FollowCountFormatter.count(data.following), "following", .following, id: "user-profile-following")
            if let cabals = data.cabals {
                item(FollowCountFormatter.count(cabals), cabals == 1 ? "cabal" : "cabals")
                    .accessibilityIdentifier("user-profile-stat-cabals")
            }
            if data.inCommon > 0 {
                Text("\(data.inCommon) in common")
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
                    .accessibilityIdentifier("user-profile-in-common")
            }
        }
        .lineLimit(1)
        .minimumScaleFactor(0.8)
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(.horizontal, MonacoTheme.Space.gutter)
    }

    private func link(_ count: String, _ noun: String, _ kind: FollowListKind, id: String) -> some View {
        NavigationLink(value: AnyAppRoute(FollowListRoute(userID: data.userID, kind: kind))) {
            item(count, noun)
        }
        .buttonStyle(.plain)
        .accessibilityIdentifier(id)
    }

    private func item(_ count: String, _ noun: String) -> some View {
        HStack(spacing: MonacoTheme.Space.xs) {
            Text(count)
                .font(MonacoTheme.Typo.captionStrong)
                .foregroundStyle(MonacoTheme.primaryText)
            Text(noun)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
        }
        .fixedSize(horizontal: false, vertical: true)
        .frame(minHeight: MonacoButtonMetrics.hitTarget)
        .contentShape(Rectangle())
        .accessibilityElement(children: .combine)
    }
}
