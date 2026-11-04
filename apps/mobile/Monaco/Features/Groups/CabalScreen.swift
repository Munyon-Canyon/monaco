import MonacoAPI
import MonacoCore
import SwiftUI

struct CabalScreen: View {
    static let sections: [any CabalSection.Type] = [
        CabalHeaderSlot.self,
        CabalClosedSlot.self,
        CabalPauseSlot.self,
        CabalJoinSlot.self,
        CabalPotSlot.self,
        CabalEmptySlot.self,
        CabalActionsSlot.self,
        CabalProposalsSlot.self,
        CabalStartersSlot.self,
        CabalTabs.self,
    ]

    static let detailsSections: [any CabalSection.Type] = [
        CabalInfoCoverSlot.self,
        CabalRulesSlot.self,
        CabalInfoLinksSlot.self,
        CabalTreasurySlot.self,
        CabalInfoFooterSlot.self,
        CabalLeaveSlot.self,
    ]

    let context: CabalContext
    let sections: [any CabalSection.Type]
    @Environment(AppEnvironment.self) private var environment
    @Environment(\.hostMainTab) private var hostMainTab
    @State private var selection: CabalTabSelection
    @State private var tabsTop: CGFloat?
    @State private var pendingMore: CabalMoreItem?
    @State private var models = CabalScreenModels()
    @State private var publicChart: ValueChartModel?
    @State private var retryTick = 0
    @State private var refresh = ScreenRefresh()
    @State private var nameBottom: CGFloat?
    @State private var barBottom: CGFloat = 0
    @State private var barWidth: CGFloat = 0
    @State private var isOnScreen = false

    init(cabalID: String, tab: CabalTab = .default, sections: [any CabalSection.Type] = Self.sections) {
        self.context = CabalContext(cabalID: cabalID)
        self.sections = sections
        _selection = State(initialValue: CabalTabSelection(tab))
    }

    static func canShare(cabal: Components.Schemas.Cabal?) -> Bool {
        guard let cabal else { return false }
        return cabal.me != nil && !CabalClosedModel.isReadOnly(cabal.status)
    }

    var body: some View {
        let sections = sections.map { $0.erased }
        let (cabal, pot) = models.prepared(cabalID: context.cabalID, environment: environment)
        let empty = models.preparedEmpty(cabalID: context.cabalID, environment: environment)
        let chart = models.preparedChart(cabalID: context.cabalID, environment: environment)
        let unread = models.preparedUnread(cabalID: context.cabalID, environment: environment)
        let showsChat = Self.showsChat(tab: selection.tab, cabal: cabal.cabal)
        Group {
            if SectionStack<CabalContext>.live(sections).isEmpty {
                NotMigratedView(screen: "Cabal")
            } else if cabal.cabal == nil, case .failed = cabal.state {
                CabalFullScreenError(identifier: "cabal-failed") { retryTick += 1 }
            } else if let loaded = cabal.cabal, loaded.me == nil, Self.showsPublicLayout(self.sections) {
                PublicCabalView(
                    cabal: cabal, returns: CabalPublicReturn(publicChart?.curve), retry: { retryTick += 1 }
                )
                .environment(\.cabalModel, cabal)
                .environment(\.cabalPotModel, pot)
                .environment(refresh)
                .task(id: retryTick) { await preparedPublicChart().refresh() }
            } else if selection.tab == .chat && !showsChat {
                Color.clear
                    .monacoCanvas()
                    .accessibilityIdentifier("cabal-opening")
            } else if showsChat {
                GroupChatView(cabalID: context.cabalID, embedded: true)
                    .id(context.cabalID)
                    .safeAreaInset(edge: .top, spacing: 0) {
                        pinnedTabs(
                            memberCount: cabal.cabal?.memberCount, chatHasUnread: false, isReadOnly: false)
                    }
            } else {
                SectionStack(context: context, sections: sections, spacing: CabalPageMetrics.spacing)
                    .environment(\.cabalModel, cabal)
                    .environment(\.cabalPotModel, pot)
                    .environment(\.cabalEmptyModel, empty)
                    .environment(\.cabalValueChart, chart)
                    .environment(\.cabalTabSelection, selection)
                    .environment(\.cabalChatUnread, unread)
                    .environment(refresh)
                    .refreshable {
                        retryTick += 1
                        await refresh.run()
                    }
            }
        }
        .onPreferenceChange(CabalHeaderNameBottom.self) { nameBottom = $0 }
        .onPreferenceChange(CabalTabsTop.self) { tabsTop = $0 }
        .overlay(alignment: .top) {
            if cabal.cabal?.me != nil && selection.tab != .chat {
                pinnedHeader(
                    memberCount: cabal.cabal?.memberCount,
                    chatHasUnread: selection.tab != .chat && unread.hasUnread,
                    isReadOnly: cabal.cabal.map { CabalClosedModel.isReadOnly($0.status) } ?? false)
            }
        }
        .cabalMoreActions(
            cabalID: context.cabalID, cabal: cabal,
            edit: models.preparedEdit(cabalID: context.cabalID, environment: environment), pending: $pendingMore
        )
        .onGeometryChange(for: CGFloat.self) {
            $0.safeAreaInsets.top
        } action: {
            barBottom = $0
        }
        .onGeometryChange(for: CGFloat.self) {
            $0.size.width
        } action: {
            barWidth = $0
        }
        .environment(\.cabalRetry, CabalRetry(tick: retryTick) { retryTick += 1 })
        .environment(
            \.cabalShare,
            CabalShare(open: { share(cabal.cabal) }, isBusy: false)
        )
        .navigationTitle(cabal.cabal?.name ?? "")
        .navigationBarTitleDisplayMode(.inline)
        .toolbar(.hidden, for: .tabBar)
        .task(id: retryTick) {
            async let loadedCabal: Void = cabal.load()
            async let loadedPot: Void = pot.load()
            async let loadedEmpty: Void = empty.load()
            _ = await (loadedCabal, loadedPot, loadedEmpty)
            let isMember = cabal.cabal?.me != nil
            async let sharedMonthRead: Void = {
                guard isMember else { return }
                await chart.load()
                await chart.observe()
            }()
            async let observedCabal: Void = cabal.observe()
            async let observedPot: Void = pot.observe()
            async let observedEmpty: Void = empty.observe()
            async let observedUnread: Void = {
                guard isMember else { return }
                await unread.load()
                await unread.observe()
            }()
            _ = await (observedCabal, observedPot, observedEmpty, sharedMonthRead, observedUnread)
        }
        .onChange(of: selection.tab, initial: true) { _, tab in
            if tab == .chat { unread.markRead() }
        }
        .onChange(of: Self.landingTab(tab: selection.tab, cabal: cabal.cabal), initial: true) { _, tab in
            if selection.tab != tab { selection.tab = tab }
        }
        .onChange(of: Self.isChatVisible(tab: selection.tab, cabal: cabal.cabal, onScreen: isOnScreen), initial: true) {
            _, visible in
            syncVisibleChat(visible)
        }
        .onScreenVisibilityChange { visible in
            isOnScreen = visible
            cabal.setVisible(visible)
            pot.setVisible(visible)
            empty.setVisible(visible)
            chart.setVisible(visible)
            unread.setVisible(visible)
        }
        .toolbar {
            if let loaded = cabal.cabal {
                ToolbarItem(placement: .principal) {
                    CabalFoldedTitle(
                        groupId: loaded.id, name: loaded.name, pictureUrl: loaded.pictureUrl,
                        memberCount: loaded.memberCount, alwaysShown: showsChat,
                        scrolledShown: CabalHeaderTitle.isShown(nameBottom: nameBottom, barBottom: barBottom),
                        maxWidth: CabalFoldedTitle.maxWidth(barWidth: barWidth))
                }
            }
            if let loaded = cabal.cabal, loaded.me == nil {
                ToolbarItem(placement: .topBarTrailing) {
                    CabalMoreMenu(items: CabalMoreItem.items(role: nil, status: loaded.status)) { pendingMore = $0 }
                }
            }
            if let loaded = cabal.cabal, loaded.me != nil {
                ToolbarItem(placement: .topBarTrailing) {
                    HStack(spacing: 0) {
                        if Self.canShare(cabal: loaded) {
                            MonacoGlassButton(
                                icon: .share, accessibilityLabel: "Share cabal", style: .quiet,
                                placement: .toolbar
                            ) { share(loaded) }
                            .frame(width: 44, height: 44)
                            .accessibilityIdentifier("cabal-share-button")
                        }
                        CabalMoreMenu(items: CabalMoreItem.items(role: loaded.me?.role, status: loaded.status)) {
                            pendingMore = $0
                        }
                        .frame(width: 44, height: 44)
                    }
                }
            }
        }
    }

    static func landingTab(tab: CabalTab, cabal: Components.Schemas.Cabal?) -> CabalTab {
        guard let cabal else { return tab }
        return tab.resolved(isReadOnly: cabal.me == nil || CabalClosedModel.isReadOnly(cabal.status))
    }

    static func isChatVisible(tab: CabalTab, cabal: Components.Schemas.Cabal?, onScreen: Bool) -> Bool {
        onScreen && showsChat(tab: tab, cabal: cabal)
    }

    private func syncVisibleChat(_ visible: Bool) {
        let navigator = environment.navigator
        if visible {
            navigator.visibleChatCabalID = context.cabalID
        } else if navigator.visibleChatCabalID == context.cabalID {
            navigator.visibleChatCabalID = nil
        }
    }

    static func showsChat(tab: CabalTab, cabal: Components.Schemas.Cabal?) -> Bool {
        guard tab == .chat, let cabal else { return false }
        return cabal.me != nil && !CabalClosedModel.isReadOnly(cabal.status)
    }

    private func pinnedTabs(memberCount: Int32?, chatHasUnread: Bool, isReadOnly: Bool) -> some View {
        CabalTabBar(
            selection: selection, memberCount: memberCount, chatHasUnread: chatHasUnread,
            isReadOnly: isReadOnly, identifierPrefix: "cabal-tab-pinned"
        )
        .monacoUnderlineTabsPinned()
        .accessibilityIdentifier("cabal-tabs-pinned")
    }

    private func pinnedHeader(memberCount: Int32?, chatHasUnread: Bool, isReadOnly: Bool) -> some View {
        let tabsShown = CabalTabsPin.isPinned(tab: selection.tab, tabsTop: tabsTop, barBottom: barBottom)
        return VStack(spacing: 0) {
            if tabsShown {
                pinnedTabs(memberCount: memberCount, chatHasUnread: chatHasUnread, isReadOnly: isReadOnly)
            }
        }
    }

    private func share(_ cabal: Components.Schemas.Cabal?) {
        guard let cabal else { return }
        ShareCardLauncher.shareCabal(
            id: cabal.id, navigator: environment.navigator, in: hostMainTab ?? environment.navigator.selectedTab)
    }

    private func preparedPublicChart() -> ValueChartModel {
        if let publicChart { return publicChart }
        let created = ValueChartModel(
            subjects: [.cabal(id: context.cabalID)], ranges: [.oneMonth], range: .oneMonth, api: environment.api,
            hints: environment.hints)
        publicChart = created
        return created
    }

    private static func showsPublicLayout(_ sections: [any CabalSection.Type]) -> Bool {
        sections.contains { $0 == CabalHeaderSlot.self }
    }
}

struct CabalFullScreenError: View {
    let identifier: String
    let retry: () -> Void

    var body: some View {
        EmptyState(
            title: CabalCopy.loadFailedTitle, message: CabalCopy.loadFailedBody,
            actionTitle: "Try again", actionIdentifier: "\(identifier)-retry",
            systemImage: MonacoIcon.offline.symbol, action: retry
        )
        .frame(maxHeight: .infinity)
        .monacoCanvas()
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier(identifier)
    }
}

@MainActor
final class CabalScreenModels {
    private var cabal: CabalModel?
    private var pot: CabalPotModel?
    private var edit: CabalEditModel?
    private var empty: CabalEmptyModel?
    private var chart: ValueChartModel?
    private var unread: CabalChatUnreadModel?

    func preparedUnread(cabalID: String, environment: AppEnvironment) -> CabalChatUnreadModel {
        let created =
            unread ?? CabalChatUnreadModel(cabalID: cabalID, api: environment.api, hints: environment.hints)
        unread = created
        return created
    }

    func preparedChart(cabalID: String, environment: AppEnvironment) -> ValueChartModel {
        let created =
            chart
            ?? ValueChartModel(
                subjects: [.cabal(id: cabalID)], ranges: [.oneMonth], range: .oneMonth, api: environment.api,
                hints: environment.hints)
        chart = created
        return created
    }

    func preparedEmpty(cabalID: String, environment: AppEnvironment) -> CabalEmptyModel {
        let created = empty ?? CabalEmptyModel(cabalID: cabalID, api: environment.api, hints: environment.hints)
        empty = created
        return created
    }

    func preparedEdit(cabalID: String, environment: AppEnvironment) -> CabalEditModel {
        let created = edit ?? CabalEditModel(cabalID: cabalID, api: environment.api, hints: environment.hints)
        edit = created
        return created
    }

    func prepared(cabalID: String, environment: AppEnvironment) -> (CabalModel, CabalPotModel) {
        let cabal = self.cabal ?? CabalModel(cabalID: cabalID, api: environment.api, hints: environment.hints)
        let pot =
            self.pot
            ?? CabalPotModel(
                cabalID: cabalID, api: environment.api, hints: environment.hints, logoStore: environment.assetLogos)
        self.cabal = cabal
        self.pot = pot
        return (cabal, pot)
    }
}

struct CabalRetry {
    var tick = 0
    var retry: (() -> Void)?
}

struct CabalShare {
    var open: () -> Void
    var isBusy = false
}

extension EnvironmentValues {
    @Entry var cabalRetry = CabalRetry()
    @Entry var cabalModel: CabalModel? = nil
    @Entry var cabalPotModel: CabalPotModel? = nil
    @Entry var cabalEmptyModel: CabalEmptyModel? = nil
    @Entry var cabalValueChart: ValueChartModel? = nil
    @Entry var cabalShare: CabalShare? = nil
    @Entry var cabalEditModel: CabalEditModel? = nil
    @Entry var cabalInfoSaving = false
}
