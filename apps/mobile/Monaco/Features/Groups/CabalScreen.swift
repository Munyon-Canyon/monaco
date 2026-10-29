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
        CabalActionsSlot.self,
        CabalProposalsSlot.self,
        CabalValueChartSlot.self,
        CabalChatPreviewSlot.self,
        CabalHoldingsSlot.self,
        CabalMemberBoardSlot.self,
        CabalActivitySlot.self,
        CabalRulesRowSlot.self,
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
    @Environment(ToastCenter.self) private var toasts
    @State private var sharing: String?
    @State private var models = CabalScreenModels()
    @State private var retryTick = 0
    @State private var refresh = ScreenRefresh()
    @State private var nameBottom: CGFloat?
    @State private var barBottom: CGFloat = 0

    init(cabalID: String, sections: [any CabalSection.Type] = Self.sections) {
        self.context = CabalContext(cabalID: cabalID)
        self.sections = sections
    }

    static func canShare(cabal: Components.Schemas.Cabal?) -> Bool {
        guard let cabal else { return false }
        return cabal.me != nil && cabal.status != "banned"
    }

    var body: some View {
        let sections = sections.map { $0.erased }
        let (cabal, pot) = models.prepared(cabalID: context.cabalID, environment: environment)
        Group {
            if SectionStack<CabalContext>.live(sections).isEmpty {
                NotMigratedView(screen: "Cabal")
            } else if cabal.cabal == nil, case .failed = cabal.state {
                CabalFullScreenError(identifier: "cabal-failed") { retryTick += 1 }
            } else if let loaded = cabal.cabal, loaded.me == nil, Self.showsPublicLayout(self.sections) {
                PublicCabalView(cabal: cabal, retry: { retryTick += 1 })
                    .environment(\.cabalModel, cabal)
                    .environment(\.cabalPotModel, pot)
                    .environment(refresh)
            } else {
                SectionStack(context: context, sections: sections)
                    .environment(\.cabalModel, cabal)
                    .environment(\.cabalPotModel, pot)
                    .environment(refresh)
                    .refreshable {
                        retryTick += 1
                        await refresh.run()
                    }
            }
        }
        .onPreferenceChange(CabalHeaderNameBottom.self) { nameBottom = $0 }
        .onGeometryChange(for: CGFloat.self) {
            $0.safeAreaInsets.top
        } action: {
            barBottom = $0
        }
        .environment(\.cabalRetry, CabalRetry(tick: retryTick) { retryTick += 1 })
        .environment(
            \.cabalShare,
            CabalShare(open: { share(cabal.cabal) }, isBusy: sharing == context.cabalID)
        )
        .navigationTitle(cabal.cabal?.name ?? "")
        .navigationBarTitleDisplayMode(.inline)
        .task(id: retryTick) {
            async let loadedCabal: Void = cabal.load()
            async let loadedPot: Void = pot.load()
            _ = await (loadedCabal, loadedPot)
            async let observedCabal: Void = cabal.observe()
            async let observedPot: Void = pot.observe()
            _ = await (observedCabal, observedPot)
        }
        .onScreenVisibilityChange { visible in
            cabal.setVisible(visible)
            pot.setVisible(visible)
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
            if let loaded = cabal.cabal, loaded.me != nil {
                ToolbarItem(placement: .topBarTrailing) {
                    HStack(spacing: MonacoTheme.Space.s) {
                        if Self.canShare(cabal: loaded) {
                            MonacoGlassButton(
                                icon: .share, accessibilityLabel: "Share cabal", style: .quiet,
                                placement: .toolbar, isBusy: sharing == context.cabalID
                            ) { share(loaded) }
                            .accessibilityIdentifier("cabal-share-button")
                        }
                        MonacoGlassButton(
                            icon: .info, accessibilityLabel: "Cabal information", style: .quiet,
                            placement: .toolbar
                        ) {
                            environment.navigator.open(
                                CabalInfoRoute(cabalID: context.cabalID),
                                in: hostMainTab ?? environment.navigator.selectedTab)
                        }
                        .accessibilityIdentifier("cabal-details-button")
                    }
                }
            }
        }
    }

    private func share(_ cabal: Components.Schemas.Cabal?) {
        guard let cabal else { return }
        Task { await ShareCardLauncher.shareCabal(cabal, api: environment.api, toasts: toasts, busy: $sharing) }
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
    @Entry var cabalShare: CabalShare? = nil
    @Entry var cabalEditModel: CabalEditModel? = nil
    @Entry var cabalInfoSaving = false
}
