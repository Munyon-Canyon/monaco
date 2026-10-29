import MonacoAPI
import MonacoCore
import SwiftUI

struct ProfileScreen: View {
    static let sections: [any ProfileSection.Type] = [
        ProfileCardSlot.self,
        ProfileStandingSlot.self,
        ProfileCashSlot.self,
        ProfileTabsSlot.self,
    ]

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(ScreenRefresh.self) private var refresh: ScreenRefresh?
    @State private var portfolio: PortfolioModel?
    @State private var own: OwnProfileModel?
    let sections: [any ProfileSection.Type]

    init(sections: [any ProfileSection.Type] = Self.sections) {
        self.sections = sections
    }

    static func canShare(profile: SessionProfile?, portfolioState: LoadState<PortfolioSummary>?) -> Bool {
        var loaded = false
        if case .loaded = portfolioState { loaded = true }
        return ShareCardAccess.canShareMe(hasProfile: profile != nil, portfolioLoaded: loaded)
    }

    static func showsLoadFailure(model: OwnProfileModel?) -> Bool {
        model?.phase == .failed && model?.card == nil
    }

    var body: some View {
        content
            .environment(portfolio)
            .environment(own)
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
                guard let model = preparedOwn() else { return }
                refresh?.register("profile-own") { await model.load() }
                await withTaskGroup(of: Void.self) { group in
                    group.addTask { await model.load() }
                    group.addTask { await model.observe() }
                }
            }
            .onScreenVisibilityChange { visible in
                portfolio?.setVisible(visible)
                own?.setVisible(visible)
                if visible, own?.phase == .loaded { Task { await own?.load() } }
            }
            .onChange(of: portfolio?.toast) { _, message in
                guard let message else { return }
                toasts.current = MonacoToast(message: message)
                portfolio?.dismissToast()
            }
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    MonacoGlassButton(
                        icon: .share, accessibilityLabel: "Share your return", style: .quiet, placement: .toolbar
                    ) {
                        share()
                    }
                    .disabled(!canShare)
                    .accessibilityIdentifier("profile-share-button")
                }
                .withoutGlass()
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

    private var canShare: Bool {
        Self.canShare(profile: own?.sessionProfile, portfolioState: portfolio?.state)
    }

    private func share() {
        ShareCardLauncher.shareMe(navigator: environment.navigator, in: .profile)
    }

    @ViewBuilder private var content: some View {
        if Self.showsLoadFailure(model: own) {
            ScrollView {
                EmptyState(
                    title: ProfileCopy.failedTitle, message: EditProfileCopy.failedMessage,
                    actionTitle: ProfileCopy.tryAgain, actionIdentifier: "profile-header-error",
                    systemImage: MonacoIcon.offline.symbol
                ) {
                    Task {
                        await own?.load()
                        await portfolio?.load()
                    }
                }
            }
            .monacoCanvas()
        } else if own?.card == nil {
            ProfileLoadingSkeleton()
        } else {
            VStack(spacing: 0) {
                OnboardingNudgeBanner()
                SectionStack(context: (), sections: sections.map { $0.erased })
            }
            .safeAreaInset(edge: .bottom, spacing: 0) { ProfileDepositDock() }
        }
    }

    private func preparedOwn() -> OwnProfileModel? {
        guard let userID = environment.viewer?.userID else { return nil }
        if let own, own.userID == userID { return own }
        let created = OwnProfileModel(api: environment.api, hints: environment.hints, viewerID: userID)
        own = created
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
