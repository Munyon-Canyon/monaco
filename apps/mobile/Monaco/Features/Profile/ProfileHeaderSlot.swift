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
    var bio: String?
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
    @Environment(\.profileHeaderPresets) private var presets

    @State private var showsShare = false

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

    private var loadFailure: some View {
        MonacoErrorRow(thing: "your profile", identifier: "profile-header-error") {
            Task { await session.reloadProfile(auth: auth) }
        }
    }

    private func identity(_ profile: SessionProfile) -> some View {
        VStack(spacing: MonacoTheme.Space.s) {
            IdentityCard(
                model: Self.model(for: profile, portfolio: portfolio?.state ?? .loading, bio: presets.bio),
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
                    IdentityShareButton(accessibilityLabel: "Share your return") { showsShare = true }
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
        .sheet(isPresented: $showsShare) {
            ShareCardHost(api: environment.api) { await $0.loadMe(profile: profile) }
                .monacoToastCenter(toasts)
        }
    }

    static func model(
        for profile: SessionProfile, portfolio: LoadState<PortfolioSummary>, bio: String?
    ) -> IdentityCardModel {
        let name = profile.displayName.trimmingCharacters(in: .whitespacesAndNewlines)
        var model = IdentityCardModel(
            displayName: name.isEmpty ? "Member" : name, handle: profile.handle,
            joinedText: MemberSinceFormatter.joined(profile.createdAt), bio: bio, positionsValue: IdentityCardCopy.zero,
            performance: .loading, periodText: IdentityCardCopy.allTime)
        switch portfolio {
        case .idle, .loading:
            break
        case .failed:
            model.performance = .failed
        case .loaded(let summary):
            model.positionsValue = summary.stats.inCabals
            model.performance = IdentityPerformance(stats: summary.stats)
        }
        return model
    }
}
