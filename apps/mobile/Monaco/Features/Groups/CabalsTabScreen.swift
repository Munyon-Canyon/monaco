import MonacoCore
import SwiftUI

struct CabalsTabScreen: View {
    static let sections: [any CabalsTabSection.Type] = [
        CabalsJoinSlot.self,
        CabalsInvitesSlot.self,
        CabalsListSlot.self,
        CabalsValueChartSlot.self,
        CabalsBoardSlot.self,
    ]

    let sections: [any CabalsTabSection.Type]
    @State private var search = CabalsSearchFocus()

    init(sections: [any CabalsTabSection.Type] = Self.sections) {
        self.sections = sections
    }

    static func visible(_ sections: [any CabalsTabSection.Type], searching: Bool) -> [any CabalsTabSection.Type] {
        guard searching else { return sections }
        return sections.filter { ObjectIdentifier($0) == ObjectIdentifier(CabalsJoinSlot.self) }
    }

    var body: some View {
        let sections = Self.visible(sections, searching: search.isSearching).map { $0.erased }
        Group {
            if SectionStack<Void>.live(sections).isEmpty {
                NotMigratedView(screen: "Cabals")
            } else {
                SectionStack(context: (), sections: sections)
            }
        }
        .environment(search)
    }
}
