import SwiftUI

struct CabalsTabScreen: View {
    static let sections: [any CabalsTabSection.Type] = [
        CabalsInvitesSlot.self,
        CabalsListSlot.self,
        CabalsJoinSlot.self,
        CabalsValueChartSlot.self,
        CabalsBoardSlot.self,
    ]

    let sections: [any CabalsTabSection.Type]

    init(sections: [any CabalsTabSection.Type] = Self.sections) {
        self.sections = sections
    }

    var body: some View {
        let sections = sections.map { $0.erased }
        if SectionStack<Void>.live(sections).isEmpty {
            NotMigratedView(screen: "Cabals")
        } else {
            SectionStack(context: (), sections: sections)
        }
    }
}
