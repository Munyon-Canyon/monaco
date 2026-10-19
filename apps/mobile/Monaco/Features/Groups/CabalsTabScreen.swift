import MonacoCore
import SwiftUI

extension EnvironmentValues {
    @Entry var cabalsFilter: Binding<String> = .constant("")
}

struct CabalsTabScreen: View {
    static let sections: [any CabalsTabSection.Type] = [
        CabalsJoinSlot.self,
        CabalsInvitesSlot.self,
        CabalsListSlot.self,
        CabalsBoardSlot.self,
    ]

    let sections: [any CabalsTabSection.Type]
    @State private var filter = ""

    init(sections: [any CabalsTabSection.Type] = Self.sections) {
        self.sections = sections
    }

    var body: some View {
        let live = SectionStack<Void>.live(sections.map { $0.erased })
        Group {
            if live.isEmpty {
                NotMigratedView(screen: "Cabals")
            } else {
                ScrollView {
                    VStack(alignment: .leading, spacing: 0) {
                        ForEach(Array(live.enumerated()), id: \.offset) { _, section in
                            AnyView(section.body(for: ()))
                        }
                    }
                    .padding(.bottom, MonacoTheme.Space.l)
                }
                .scrollDismissesKeyboard(.interactively)
            }
        }
        .environment(\.cabalsFilter, $filter)
    }
}
