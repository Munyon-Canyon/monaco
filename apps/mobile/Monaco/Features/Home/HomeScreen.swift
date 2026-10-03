import SwiftUI

struct HomeScreen: View {
    static let sections: [any HomeSection.Type] = [
        HomeNudgeSlot.self,
        HomeBalanceSlot.self,
        HomePendingVotesSlot.self,
        HomePortfolioSlot.self,
        HomePeopleBoardSlot.self,
    ]

    let sections: [any HomeSection.Type]

    init(sections: [any HomeSection.Type] = Self.sections) {
        self.sections = sections
    }

    var body: some View {
        let sections = sections.map { $0.erased }
        let live = SectionStack<Void>.live(sections)
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
}
