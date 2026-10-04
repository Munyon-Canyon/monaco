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
