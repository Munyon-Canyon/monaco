import MonacoCore
import SwiftUI

struct HomeScreen: View {
    static let sections: [any HomeSection.Type] = [
        HomeNudgeSlot.self,
        HomePortfolioSlot.self,
        HomeBalanceSlot.self,
        HomePendingVotesSlot.self,
        HomeCabalsSlot.self,
        HomePeopleBoardSlot.self,
    ]

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(ScreenRefresh.self) private var refresh: ScreenRefresh?
    @State private var portfolio: PortfolioModel?
    @State private var reads = HomeReads()
    let sections: [any HomeSection.Type]

    init(sections: [any HomeSection.Type] = Self.sections) {
        self.sections = sections
    }

    var body: some View {
        let sections = sections.map { $0.erased }
        let live = SectionStack<Void>.live(sections)
        Group {
            if live.contains(where: { ObjectIdentifier($0) != ObjectIdentifier(HomeNudgeSlot.self) }) {
                SectionStack(context: (), sections: sections)
                    .environment(portfolio)
                    .environment(\.homeReads, reads)
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
            ToolbarItem(placement: .topBarTrailing) {
                profileButton
            }
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
