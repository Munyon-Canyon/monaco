import MonacoAPI
import MonacoCore
import SwiftUI

struct ProfileScreen: View {
    static let sections: [any ProfileSection.Type] = [
        ProfileHeaderSlot.self,
        ProfileFollowCountsSlot.self,
        ProfileBalanceSlot.self,
        ProfileCabalsSlot.self,
        ProfileLinksSlot.self,
    ]

    @EnvironmentObject private var auth: PrivyAuthService
    @Environment(AppEnvironment.self) private var environment
    @Environment(AppSessionStore.self) private var session
    @Environment(ToastCenter.self) private var toasts
    @Environment(ScreenRefresh.self) private var refresh: ScreenRefresh?
    @State private var portfolio: PortfolioModel?
    @State private var selfProfile: UserProfileModel?
    let sections: [any ProfileSection.Type]

    init(sections: [any ProfileSection.Type] = Self.sections) {
        self.sections = sections
    }

    static func canShare(profile: SessionProfile?, portfolioState: LoadState<PortfolioSummary>?) -> Bool {
        var loaded = false
        if case .loaded = portfolioState { loaded = true }
        return ShareCardAccess.canShareMe(hasProfile: profile != nil, portfolioLoaded: loaded)
    }

    static func showsLoadFailure(profile: SessionProfile?, isLoading: Bool) -> Bool {
        profile == nil && !isLoading
    }

    var body: some View {
        content
            .environment(portfolio)
            .environment(selfProfile)
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .task {
                let portfolio = preparedPortfolio()
                refresh?.register("profile-portfolio") { await portfolio.load() }
                await withTaskGroup(of: Void.self) { group in
                    group.addTask { await portfolio.load() }
                    group.addTask { await portfolio.observe() }
                }
            }
            .task(id: environment.viewer?.userID) {
                guard let model = preparedSelfProfile() else { return }
                refresh?.register("profile-follow-counts") { await model.refresh() }
                if model.profile == nil {
                    await model.load()
                } else {
                    await model.refresh()
                }
            }
            .onScreenVisibilityChange { portfolio?.setVisible($0) }
            .onChange(of: portfolio?.toast) { _, message in
                guard let message else { return }
                toasts.current = MonacoToast(message: message)
                portfolio?.dismissToast()
            }
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    MonacoGlassButton(
                        icon: .settings, accessibilityLabel: "Settings", style: .quiet, placement: .toolbar
                    ) {
                        environment.navigator.open(SettingsRoute(), in: .profile)
                    }
                    .accessibilityIdentifier("profile-settings-row")
                }
                .withoutGlass()
            }
    }

    @ViewBuilder private var content: some View {
        if Self.showsLoadFailure(profile: session.profile, isLoading: session.isLoading) {
            ScrollView {
                EmptyState(
                    title: EditProfileCopy.failedTitle, message: EditProfileCopy.failedMessage,
                    actionTitle: "Try again", actionIdentifier: "profile-header-error",
                    systemImage: MonacoIcon.offline.symbol
                ) {
                    Task {
                        await session.reloadProfile(auth: auth)
                        await portfolio?.load()
                    }
                }
            }
            .monacoCanvas()
        } else if session.profile == nil {
            ProfileLoadingSkeleton()
        } else {
            SectionStack(context: (), sections: sections.map { $0.erased })
        }
    }

    private func preparedSelfProfile() -> UserProfileModel? {
        guard let userID = environment.viewer?.userID else { return nil }
        if let selfProfile, selfProfile.userID == userID { return selfProfile }
        let created = UserProfileModel(userID: userID, api: environment.api)
        selfProfile = created
        return created
    }

    private func preparedPortfolio() -> PortfolioModel {
        if let portfolio { return portfolio }
        let created = PortfolioModel(api: environment.api, hints: environment.hints)
        portfolio = created
        return created
    }
}

private struct ProfileLoadingSkeleton: View {
    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                SkeletonBar(width: 168)
                SkeletonCardBlock()
                MonacoRowSkeleton(rows: 4, markShape: .circle)
                    .padding(.horizontal, -MonacoTheme.Space.gutter)
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
        }
        .monacoCanvas()
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("Loading your profile")
        .accessibilityIdentifier("profile-header-loading")
    }
}

extension ToolbarContent {
    @ToolbarContentBuilder
    fileprivate func withoutGlass() -> some ToolbarContent {
        if #available(iOS 26.0, *) {
            sharedBackgroundVisibility(.hidden)
        } else {
            self
        }
    }
}
