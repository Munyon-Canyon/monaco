import MonacoAPI
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
    @State private var showsShare = false
    let sections: [any ProfileSection.Type]

    init(sections: [any ProfileSection.Type] = Self.sections) {
        self.sections = sections
    }

    static func canShare(profile: SessionProfile?, portfolioState: LoadState<PortfolioSummary>?) -> Bool {
        var loaded = false
        if case .loaded = portfolioState { loaded = true }
        return ShareCardAccess.canShareMe(hasProfile: profile != nil, portfolioLoaded: loaded)
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
            .toolbar {
                if Self.canShare(profile: environment.sessionStore.profile, portfolioState: portfolio?.state) {
                    ToolbarItem(placement: .topBarTrailing) {
                        Button {
                            showsShare = true
                        } label: {
                            Image(systemName: "square.and.arrow.up")
                                .frame(width: 44, height: 44)
                                .contentShape(Rectangle())
                        }
                        .accessibilityLabel("Share your return")
                        .accessibilityIdentifier("profile-share-button")
                    }
                }
            }
            .sheet(isPresented: $showsShare) {
                if let profile = environment.sessionStore.profile {
                    ShareCardHost(api: environment.api) { await $0.loadMe(profile: profile) }
                        .monacoToastCenter(toasts)
                }
            }
    }

    private func preparedPortfolio() -> PortfolioModel {
        if let portfolio { return portfolio }
        let created = PortfolioModel(api: environment.api, hints: environment.hints)
        portfolio = created
        return created
    }
}
