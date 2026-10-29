import MonacoCore
import SwiftUI

enum HomeScreenMetrics {
    static let scrolledOffset: CGFloat = 1
}

struct HomeScreen: View {
    static let sections: [any HomeSection.Type] = [
        HomeFigureSlot.self,
        HomePendingVotesSlot.self,
        HomeChartSlot.self,
        HomeBalanceSlot.self,
        HomeFirstUseSlot.self,
        HomeCabalsSlot.self,
        HomeLeaderboardSlot.self,
        HomeEveryonePreviewSlot.self,
        HomeFeedPreviewSlot.self,
    ]

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(ScreenRefresh.self) private var refresh: ScreenRefresh?
    @State private var portfolio: PortfolioModel?
    @State private var reads = HomeReads()
    @State private var voteNeeds = HomeVoteNeeds()
    @State private var scrolled = false
    let sections: [any HomeSection.Type]

    init(sections: [any HomeSection.Type] = Self.sections) {
        self.sections = sections
    }

    var body: some View {
        let sections = sections.map { $0.erased }
        let live = SectionStack<Void>.live(sections)
        Group {
            if live.isEmpty {
                NotMigratedView(screen: "Home")
                    .monacoCanvas()
            } else {
                HomeSectionStack(sections: sections, scrolled: $scrolled)
                    .environment(portfolio)
                    .environment(\.homeReads, reads)
                    .environment(\.homeVoteNeeds, voteNeeds)
                    .environment(\.homeCashEmphasis, portfolio?.summary?.isFirstUse == true ? .quiet : .prominent)
            }
        }
        .toolbar {
            ToolbarItem(placement: scrolled ? .principal : .topBarLeading) {
                MonacoWordmark(.toolbar, isTitle: true)
                    .fixedSize()
                    .accessibilityIdentifier("home-wordmark")
            }
            .withoutGlass()
            ToolbarItem(placement: .topBarTrailing) {
                HomeBellButton()
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
            refresh?.register("home-portfolio-chart") { await portfolio.chart.refresh() }
            await portfolio.load()
            let loadsChart = portfolio.summary.map { !$0.isFirstUse } ?? true
            await withTaskGroup(of: Void.self) { group in
                group.addTask { await portfolio.observe() }
                if loadsChart {
                    group.addTask { await portfolio.chart.load() }
                    group.addTask { await portfolio.chart.observe() }
                }
            }
        }
        .onScreenVisibilityChange {
            portfolio?.setVisible($0)
            portfolio?.chart.setVisible($0)
        }
        .onChange(of: portfolio?.chart.toast) { _, message in
            guard let message else { return }
            toasts.current = MonacoToast(message: message)
            portfolio?.chart.dismissToast()
        }
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

private struct HomeSectionStack: View {
    let sections: [any ScreenSection<Void>.Type]
    @Binding var scrolled: Bool
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
            .onScrollGeometryChange(for: Bool.self) {
                $0.contentOffset.y + $0.contentInsets.top > HomeScreenMetrics.scrolledOffset
            } action: { _, isScrolled in
                scrolled = isScrolled
            }
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
