import SwiftUI

struct CabalScreen: View {
    static let sections: [any CabalSection.Type] = [
        CabalHeaderSlot.self,
        CabalPauseSlot.self,
        CabalJoinSlot.self,
        CabalPotSlot.self,
        CabalProposalsSlot.self,
        CabalMemberBoardSlot.self,
        CabalActivitySlot.self,
        CabalChatSlot.self,
        CabalCashOutSlot.self,
    ]

    static let detailsSections: [any CabalSection.Type] = [
        CabalInviteCodeSlot.self,
        CabalInviteMemberSlot.self,
        CabalTreasurySlot.self,
        CabalEditSlot.self,
        CabalLeaveSlot.self,
    ]

    let context: CabalContext
    let sections: [any CabalSection.Type]
    let detailsSections: [any CabalSection.Type]
    @State private var showsDetails = false

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
        .toolbar {
            if !SectionStack<CabalContext>.live(details).isEmpty {
                ToolbarItem(placement: .topBarTrailing) {
                    Button("Details") {
                        showsDetails = true
                    }
                    .accessibilityIdentifier("cabalDetailsButton")
                }
            }
        }
        .sheet(isPresented: $showsDetails) {
            SectionStack(context: context, sections: details)
        }
    }
}
