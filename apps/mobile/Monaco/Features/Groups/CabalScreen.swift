import MonacoAPI
import MonacoCore
import SwiftUI

struct CabalScreen: View {
    static let sections: [any CabalSection.Type] = [
        CabalHeaderSlot.self,
        CabalPotSlot.self,
        CabalValueChartSlot.self,
        CabalSliceSlot.self,
        CabalPauseSlot.self,
        CabalJoinSlot.self,
        CabalActionsSlot.self,
        CabalProposalsSlot.self,
        CabalHoldingsSlot.self,
        CabalAgentSlot.self,
        CabalMemberBoardSlot.self,
        CabalActivitySlot.self,
    ]

    static let detailsSections: [any CabalSection.Type] = [
        CabalInviteMemberSlot.self,
        CabalRulesSlot.self,
        CabalTreasurySlot.self,
        CabalLeaveSlot.self,
    ]

    private static let heroScrollDistance: CGFloat = 140

    let context: CabalContext
    let sections: [any CabalSection.Type]
    let detailsSections: [any CabalSection.Type]
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @State private var showsDetails = false
    @State private var heroScrolledAway = false
    @State private var models = CabalScreenModels()
    @State private var retryTick = 0
    @State private var refresh = ScreenRefresh()

    init(
        cabalID: String,
        sections: [any CabalSection.Type] = Self.sections,
        detailsSections: [any CabalSection.Type] = Self.detailsSections
    ) {
        self.context = CabalContext(cabalID: cabalID)
        self.sections = sections
        self.detailsSections = detailsSections
    }

    var body: some View {
        let sections = sections.map { $0.erased }
        let details = detailsSections.map { $0.erased }
        let (cabal, pot) = models.prepared(cabalID: context.cabalID, environment: environment)
        Group {
            if SectionStack<CabalContext>.live(sections).isEmpty {
                NotMigratedView(screen: "Cabal")
            } else if cabal.cabal == nil, case .failed = cabal.state {
                ScrollView {
                    MonacoErrorRow(thing: "this cabal", identifier: "cabal-failed") { retryTick += 1 }
                        .padding(.top, MonacoTheme.Space.m)
                }
                .monacoCanvas()
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
        .environment(\.cabalRetry, CabalRetry(tick: retryTick) { retryTick += 1 })
        .onScrollGeometryChange(for: Bool.self) { geometry in
            geometry.contentOffset.y + geometry.contentInsets.top > Self.heroScrollDistance
        } action: { _, scrolledAway in
            heroScrolledAway = scrolledAway
        }
        .navigationTitle(heroScrolledAway ? cabal.cabal?.name ?? "" : "")
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
            if !SectionStack<CabalContext>.live(details).isEmpty {
                ToolbarItem(placement: .topBarTrailing) {
                    Button {
                        showsDetails = true
                    } label: {
                        Image(systemName: "info.circle")
                            .frame(width: 44, height: 44)
                            .contentShape(Rectangle())
                    }
                    .accessibilityLabel("Cabal details")
                    .accessibilityIdentifier("cabal-details-button")
                }
            }
        }
        .sheet(isPresented: $showsDetails) {
            NavigationStack {
                SectionStack(context: context, sections: details)
                    .environment(\.cabalModel, cabal)
                    .environment(\.cabalPotModel, pot)
                    .monacoSheet(title: "Cabal details")
                    .navigationBarTitleDisplayMode(.inline)
                    .toolbar {
                        ToolbarItem(placement: .topBarTrailing) {
                            Button("Done") {
                                showsDetails = false
                            }
                            .accessibilityIdentifier("cabal-details-done")
                        }
                    }
                    .navigationDestination(for: AnyAppRoute.self) { $0.destination() }
            }
            .presentationDetents([.medium, .large])
            .monacoToastCenter(toasts)
        }
    }

}

@MainActor
private final class CabalScreenModels {
    private var cabal: CabalModel?
    private var pot: CabalPotModel?

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

extension EnvironmentValues {
    @Entry var cabalRetry = CabalRetry()
    @Entry var cabalModel: CabalModel? = nil
    @Entry var cabalPotModel: CabalPotModel? = nil
}
