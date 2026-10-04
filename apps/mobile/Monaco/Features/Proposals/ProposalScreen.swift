import MonacoCore
import SwiftUI

struct ProposalScreen: View {
    static let sections: [any ProposalSection.Type] = [
        ProposalDetailSlot.self,
        ProposalCommentsSlot.self,
    ]

    let context: ProposalContext
    let sections: [any ProposalSection.Type]
    @State private var refresh = ScreenRefresh()

    init(proposalID: String, sections: [any ProposalSection.Type] = Self.sections) {
        self.context = ProposalContext(proposalID: proposalID)
        self.sections = sections
    }

    var body: some View {
        let sections = sections.map { $0.erased }
        if SectionStack<ProposalContext>.live(sections).isEmpty {
            NotMigratedView(screen: "Proposal")
        } else {
            SectionStack(context: context, sections: sections)
                .environment(refresh)
                .refreshable { await refresh.run() }
        }
    }
}
