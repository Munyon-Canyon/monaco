import MonacoAPI
import MonacoCore
import SwiftUI

enum ProfileHeaderSlot: ProfileSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        ProfileHeader()
    }
}

struct ProfileHeaderPresets {
    var showsFacePicker = false
}

enum IdentityOneMonth: Equatable {
    case loading
    case failed
    case loaded(PublicPerformance)

    init(_ model: UserProfileModel) {
        if let performance = model.performance {
            self = .loaded(performance)
        } else if model.phase == .failed || model.phase == .unavailable {
            self = .failed
        } else {
            self = .loading
        }
    }
}

extension EnvironmentValues {
    @Entry var profileHeaderPresets = ProfileHeaderPresets()
}

struct ProfileHeader: View {
    @EnvironmentObject private var auth: PrivyAuthService
    @Environment(AppEnvironment.self) private var environment
    @Environment(AppSessionStore.self) private var session
    @Environment(ToastCenter.self) private var toasts
    @Environment(ScreenRefresh.self) private var refresh: ScreenRefresh?
    @Environment(PortfolioModel.self) private var portfolio: PortfolioModel?
    @Environment(UserProfileModel.self) private var selfProfile: UserProfileModel?
    @Environment(\.profileHeaderPresets) private var presets

    @State private var sharing: String?

    var body: some View {
        VStack(spacing: 0) {
            if let profile = session.profile {
                identity(profile)
            } else if session.isLoading {
                IdentityCardSkeleton(identifier: "profile-header-loading", label: "Loading your profile")
                    .padding(.horizontal, MonacoTheme.Space.gutter)
            } else {
                loadFailure
            }
        }
        .task {
            refresh?.register("profile-header") { [session, auth] in
                await session.reloadProfile(auth: auth)
            }
        }
    }

    private var oneMonth: IdentityOneMonth? {
        if let selfProfile { return IdentityOneMonth(selfProfile) }
        return environment.viewer == nil ? nil : .loading
    }

    private var loadFailure: some View {
        MonacoErrorRow(thing: "your profile", identifier: "profile-header-error") {
            Task { await session.reloadProfile(auth: auth) }
        }
    }

    private func identity(_ profile: SessionProfile) -> some View {
        VStack(spacing: MonacoTheme.Space.s) {
            IdentityCard(
                model: Self.model(
                    for: profile, portfolio: portfolio?.state ?? .loading, oneMonth: oneMonth),
                editRoute: AnyAppRoute(EditProfileRoute())
            ) {
                ProfilePhotoPicker(
                    auth: auth, size: 64, badge: .pencil(ring: MonacoTheme.identityFill), avatarRing: .identity,
                    initiallyOpen: presets.showsFacePicker
                ) {
                    toasts.current = $0
                }
            } share: {
                if ProfileScreen.canShare(profile: profile, portfolioState: portfolio?.state) {
                    IdentityShareButton(
                        accessibilityLabel: "Share your return", isBusy: sharing == ShareCardLauncher.meKey
                    ) {
                        Task {
                            await ShareCardLauncher.shareMe(
                                profile: profile, api: environment.api, toasts: toasts, busy: $sharing)
                        }
                    }
                    .accessibilityIdentifier("profile-share-button")
                }
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            if case .failed = portfolio?.state {
                MonacoErrorRow(thing: "your portfolio", identifier: "profile-stats-retry") {
                    Task { await portfolio?.load() }
                }
            }
        }
    }

    static func model(
        for profile: SessionProfile, portfolio: LoadState<PortfolioSummary>, oneMonth: IdentityOneMonth? = nil
    ) -> IdentityCardModel {
        let name = profile.displayName.trimmingCharacters(in: .whitespacesAndNewlines)
        var model = IdentityCardModel(
            displayName: name.isEmpty ? "Member" : name, handle: profile.handle,
            joinedText: MemberSinceFormatter.joined(profile.createdAt), bio: profile.bio,
            positionsValue: IdentityCardCopy.zero, performance: .loading, periodText: IdentityCardCopy.allTime,
            positionsPhase: .loading)
        switch portfolio {
        case .idle, .loading:
            break
        case .failed:
            model.performance = .failed
            model.positionsPhase = .failed
        case .loaded(let summary):
            model.positionsValue = summary.stats.inCabals
            model.performance = IdentityPerformance(stats: summary.stats)
            model.positionsPhase = .loaded
        }
        guard let oneMonth else { return model }
        model.periodText = PublicPerformance.periodLabel
        switch oneMonth {
        case .loading: model.performance = .loading
        case .failed: model.performance = .failed
        case .loaded(let performance): model.performance = IdentityPerformance(performance)
        }
        return model
    }
}
