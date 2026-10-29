import MonacoAPI
import MonacoCore
import SwiftUI

extension EnvironmentValues {
    @Entry var cabalTabSelection: CabalTabSelection? = nil
}

struct CabalTabsTop: PreferenceKey {
    static let defaultValue: CGFloat? = nil

    static func reduce(value: inout CGFloat?, nextValue: () -> CGFloat?) {
        value = nextValue() ?? value
    }
}

enum CabalTabsPin {
    static func isPinned(tabsTop: CGFloat?, barBottom: CGFloat) -> Bool {
        guard let tabsTop else { return false }
        return tabsTop <= barBottom
    }
}

enum CabalTabs: CabalSection {
    static let isLive = true

    static func sections(for tab: CabalTab) -> [any CabalSection.Type] {
        switch tab {
        case .treasury: [CabalHoldingsSlot.self, CabalValueChartSlot.self, CabalActivitySlot.self]
        case .rules: [CabalRulesRowSlot.self]
        case .members: [CabalMemberBoardSlot.self]
        case .voters: [CabalVoterBoardSlot.self]
        }
    }

    static func body(for context: CabalContext) -> some View {
        CabalTabsLive(context: context)
    }
}

struct CabalTabBar: View {
    @Bindable var selection: CabalTabSelection
    let memberCount: Int32?
    var identifierPrefix = "cabal-tab"

    var body: some View {
        MonacoUnderlineTabs(
            CabalTab.allCases, selection: $selection.tab,
            count: { $0.count(memberCount: memberCount) },
            identifier: { "\(identifierPrefix)-\($0.rawValue)" },
            label: { $0.title }
        )
    }
}

private struct CabalTabsLive: View {
    let context: CabalContext

    @Environment(\.cabalTabSelection) private var shared
    @Environment(\.cabalModel) private var cabalModel
    @State private var local = CabalTabSelection()

    var body: some View {
        let selection = shared ?? local
        VStack(alignment: .leading, spacing: SectionStackMetrics.spacing) {
            CabalTabBar(selection: selection, memberCount: cabalModel?.cabal?.memberCount)
                .background {
                    GeometryReader { proxy in
                        Color.clear.preference(key: CabalTabsTop.self, value: proxy.frame(in: .global).minY)
                    }
                }
                .accessibilityElement(children: .contain)
                .accessibilityIdentifier("cabal-tabs")
            SectionStackLayout(spacing: SectionStackMetrics.spacing) {
                ForEach(Array(CabalTabs.sections(for: selection.tab).enumerated()), id: \.offset) { _, section in
                    AnyView(section.body(for: context))
                }
            }
            .id(selection.tab)
        }
    }
}
