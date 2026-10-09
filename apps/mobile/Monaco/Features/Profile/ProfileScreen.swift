import MonacoCore
import SwiftUI

struct ProfileScreen: View {
    static let sections: [any ProfileSection.Type] = [
        ProfileHeaderSlot.self,
        ProfileFollowCountsSlot.self,
        ProfileStatsSlot.self,
        ProfileBalanceSlot.self,
        ProfileCabalsSlot.self,
        ProfileLinksSlot.self,
    ]

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(ScreenRefresh.self) private var refresh: ScreenRefresh?
    @State private var portfolio: PortfolioModel?
    let sections: [any ProfileSection.Type]

    init(sections: [any ProfileSection.Type] = Self.sections) {
        self.sections = sections
    }

    var body: some View {
        SectionStack(context: (), sections: sections.map { $0.erased })
            .environment(portfolio)
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .task {
                let portfolio = preparedPortfolio()
                refresh?.register("profile-portfolio") { await portfolio.load() }
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
}
