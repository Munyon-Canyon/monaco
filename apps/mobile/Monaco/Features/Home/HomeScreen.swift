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
        let sections = sections.map { $0 as! any ScreenSection<Void>.Type }
        if SectionStack<Void>.live(sections).isEmpty {
            NotMigratedView(screen: "Home")
        } else {
            SectionStack(context: (), sections: sections)
        }
    }
}
