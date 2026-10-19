import MonacoCore
import SwiftUI

struct HomeScreen: View {
    static let sections: [any HomeSection.Type] = [
        HomeNudgeSlot.self,
        HomePortfolioSlot.self,
        HomeBalanceSlot.self,
        HomePendingVotesSlot.self,
        HomeCabalsSlot.self,
        HomeLeaderboardSlot.self,
        HomeMarketUpdatesSlot.self,
    ]

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(ScreenRefresh.self) private var refresh: ScreenRefresh?
    @State private var portfolio: PortfolioModel?
    @State private var reads = HomeReads()
    @State private var voteNeeds = HomeVoteNeeds()
    let sections: [any HomeSection.Type]

    init(sections: [any HomeSection.Type] = Self.sections) {
        self.sections = sections
    }

    var body: some View {
        let sections = sections.map { $0.erased }
        let live = SectionStack<Void>.live(sections)
        Group {
            if live.contains(where: { ObjectIdentifier($0) != ObjectIdentifier(HomeNudgeSlot.self) }) {
                HomeSectionStack(sections: sections)
                    .environment(portfolio)
                    .environment(\.homeReads, reads)
                    .environment(\.homeVoteNeeds, voteNeeds)
            } else {
                VStack(spacing: 0) {
                    if !live.isEmpty {
                        OnboardingNudgeBanner()
                            .padding(.top, MonacoTheme.Space.m)
                    }
                    NotMigratedView(screen: "Home")
                }
                .monacoCanvas()
            }
        }
        .toolbar {
            ToolbarItem(placement: .topBarLeading) {
                MonacoWordmark(.toolbar, isTitle: true)
                    .fixedSize()
            }
            .withoutGlass()
            ToolbarItem(placement: .topBarTrailing) {
                notificationsButton
            }
            .withoutGlass()
            ToolbarItem(placement: .topBarTrailing) {
                profileButton
            }
            .withoutGlass()
        }
        .task {
            let portfolio = preparedPortfolio()
            refresh?.register("home-portfolio") { await portfolio.load() }
            await withTaskGroup(of: Void.self) { group in
                group.addTask { await portfolio.load() }
                group.addTask { await portfolio.observe() }
            }
        }
        .onScreenVisibilityChange { portfolio?.setVisible($0) }
        .onChange(of: portfolio?.toast) { _, message in
            guard let message else { return }
            toasts.current = MonacoToast(message: message)
            portfolio?.dismissToast()
        }
    }

    private func preparedPortfolio() -> PortfolioModel {
        if let portfolio { return portfolio }
        let created = PortfolioModel(api: environment.api, hints: environment.hints)
        portfolio = created
        return created
    }

    private var notificationsButton: some View {
        MonacoGlassButton(
            systemImage: MonacoIcon.bell.symbol, accessibilityLabel: "Notifications", style: .quiet,
            placement: .toolbar
        ) {
            environment.navigator.open(ActivityFeedRoute(), in: .home)
        }
        .accessibilityIdentifier("home-notifications-button")
    }

    private var profileButton: some View {
        let profile = environment.sessionStore.profile
        return Button {
            environment.navigator.selectedTab = .profile
        } label: {
            MonacoAvatar(
                photoURL: profile?.photoURL?.absoluteString,
                displayName: profile?.displayName ?? "",
                size: 32,
                seed: environment.viewer?.userID
            )
            .frame(width: 44, height: 44)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityLabel("Profile")
        .accessibilityIdentifier("home-profile-button")
    }
}

private struct HomeSectionStack: View {
    let sections: [any ScreenSection<Void>.Type]
    @Environment(\.insideMonacoSheet) private var insideSheet

    var body: some View {
        ScrollViewReader { proxy in
            ScrollView {
                SectionStackLayout(spacing: MonacoTheme.Space.section) {
                    ForEach(Array(SectionStack<Void>.live(sections).enumerated()), id: \.offset) { _, section in
                        AnyView(section.body(for: ()))
                    }
                }
            }
            .contentMargins(.bottom, MonacoTheme.Space.l, for: .scrollContent)
            .scrollDismissesKeyboard(.interactively)
            .environment(\.sectionScrollProxy, proxy)
            .background { if !insideSheet { MonacoCanvasBackground() } }
        }
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
