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
        CabalInviteCodeSlot.self,
        CabalInviteMemberSlot.self,
        CabalRulesSlot.self,
        CabalTreasurySlot.self,
        CabalEditSlot.self,
        CabalLeaveSlot.self,
    ]

    private static let heroScrollDistance: CGFloat = 140

    let context: CabalContext
    let sections: [any CabalSection.Type]
    let detailsSections: [any CabalSection.Type]
    @Environment(AppEnvironment.self) private var environment
    @State private var showsDetails = false
    @State private var heroScrolledAway = false
    @State private var titleModel: CabalActionsModel?

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
        Group {
            if SectionStack<CabalContext>.live(sections).isEmpty {
                NotMigratedView(screen: "Cabal")
            } else {
                SectionStack(context: context, sections: sections)
            }
        }
        .onScrollGeometryChange(for: Bool.self) { geometry in
            geometry.contentOffset.y + geometry.contentInsets.top > Self.heroScrollDistance
        } action: { _, scrolledAway in
            heroScrolledAway = scrolledAway
        }
        .navigationTitle(heroScrolledAway ? titleModel?.cabalName ?? "" : "")
        .navigationBarTitleDisplayMode(.inline)
        .task {
            let model = preparedTitleModel()
            await model.load()
            await model.observe()
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
                    .navigationTitle("Cabal details")
                    .navigationBarTitleDisplayMode(.inline)
                    .toolbar {
                        ToolbarItem(placement: .topBarTrailing) {
                            Button("Done") {
                                showsDetails = false
                            }
                            .accessibilityIdentifier("cabal-details-done")
                        }
                    }
            }
            .presentationDetents([.medium, .large])
        }
    }

    private func preparedTitleModel() -> CabalActionsModel {
        if let titleModel { return titleModel }
        let created = CabalActionsModel(cabalID: context.cabalID, api: environment.api, hints: environment.hints)
        titleModel = created
        return created
    }
}
