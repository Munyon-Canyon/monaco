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
        CabalSliceSlot.self,
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

    init(cabalID: String, tab: CabalTab = .default, sections: [any CabalSection.Type] = Self.sections) {
        self.context = CabalContext(cabalID: cabalID)
        self.sections = sections
        _selection = State(initialValue: CabalTabSelection(tab))
    }

    static func canShare(cabal: Components.Schemas.Cabal?) -> Bool {
        guard let cabal else { return false }
        return cabal.me != nil && cabal.status != "banned"
    }

    var body: some View {
        let sections = sections.map { $0.erased }
        let (cabal, pot) = models.prepared(cabalID: context.cabalID, environment: environment)
        let empty = models.preparedEmpty(cabalID: context.cabalID, environment: environment)
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
            } else {
                SectionStack(context: context, sections: sections)
                    .environment(\.cabalModel, cabal)
                    .environment(\.cabalPotModel, pot)
                    .environment(\.cabalEmptyModel, empty)
                    .environment(\.cabalTabSelection, selection)
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
            if CabalTabsPin.isPinned(tabsTop: tabsTop, barBottom: barBottom), cabal.cabal?.me != nil {
                VStack(spacing: 0) {
                    CabalMoveReader(cabalID: context.cabalID) { move in
                        CabalPotBand(model: pot, move: move, isPinned: true)
                            .background(MonacoTheme.canvas)
                            .accessibilityIdentifier("cabal-pot-pinned")
                    }
                    CabalTabBar(
                        selection: selection, memberCount: cabal.cabal?.memberCount,
                        identifierPrefix: "cabal-tab-pinned"
                    )
                    .monacoUnderlineTabsPinned()
                    .accessibilityIdentifier("cabal-tabs-pinned")
                }
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
        .environment(\.cabalRetry, CabalRetry(tick: retryTick) { retryTick += 1 })
        .environment(
            \.cabalShare,
            CabalShare(open: { share(cabal.cabal) }, isBusy: false)
        )
        .navigationTitle(cabal.cabal?.name ?? "")
        .navigationBarTitleDisplayMode(.inline)
        .task(id: retryTick) {
            async let loadedCabal: Void = cabal.load()
            async let loadedPot: Void = pot.load()
            async let loadedEmpty: Void = empty.load()
            _ = await (loadedCabal, loadedPot, loadedEmpty)
            async let observedCabal: Void = cabal.observe()
            async let observedPot: Void = pot.observe()
            async let observedEmpty: Void = empty.observe()
            _ = await (observedCabal, observedPot, observedEmpty)
        }
        .onScreenVisibilityChange { visible in
            cabal.setVisible(visible)
            pot.setVisible(visible)
            empty.setVisible(visible)
        }
        .toolbar {
            if let loaded = cabal.cabal {
                ToolbarItem(placement: .principal) {
                    let shown = CabalHeaderTitle.isShown(nameBottom: nameBottom, barBottom: barBottom)
                    Text(CabalHeaderCopy.shortName(loaded.name))
                        .font(MonacoTheme.Typo.section)
                        .foregroundStyle(MonacoTheme.primaryText)
                        .lineLimit(1)
                        .opacity(shown ? 1 : 0)
                        .animation(.easeOut(duration: 0.15), value: shown)
                        .accessibilityHidden(!shown)
                        .accessibilityIdentifier("cabal-inline-title")
                }
            }
            if let loaded = cabal.cabal, loaded.me == nil {
                ToolbarItem(placement: .topBarTrailing) {
                    CabalMoreMenu(items: CabalMoreItem.items(role: nil, status: loaded.status)) { pendingMore = $0 }
                }
            }
            if let loaded = cabal.cabal, loaded.me != nil {
                ToolbarItem(placement: .topBarTrailing) {
                    HStack(spacing: MonacoTheme.Space.s) {
                        if Self.canShare(cabal: loaded) {
                            MonacoGlassButton(
                                icon: .share, accessibilityLabel: "Share cabal", style: .quiet,
                                placement: .toolbar
                            ) { share(loaded) }
                            .accessibilityIdentifier("cabal-share-button")
                        }
                        CabalMoreMenu(items: CabalMoreItem.items(role: loaded.me?.role, status: loaded.status)) {
                            pendingMore = $0
                        }
                    }
                }
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
    @Entry var cabalShare: CabalShare? = nil
    @Entry var cabalEditModel: CabalEditModel? = nil
    @Entry var cabalInfoSaving = false
}
