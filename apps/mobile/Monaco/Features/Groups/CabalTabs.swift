import MonacoAPI
import MonacoCore
import SwiftUI

extension EnvironmentValues {
    @Entry var cabalTabSelection: CabalTabSelection? = nil
    @Entry var cabalChatUnread: CabalChatUnreadModel? = nil
}

struct CabalTabsTop: PreferenceKey {
    static let defaultValue: CGFloat? = nil

    static func reduce(value: inout CGFloat?, nextValue: () -> CGFloat?) {
        value = nextValue() ?? value
    }
}

enum CabalPageMetrics {
    static let spacing = MonacoTheme.Space.sm
    static let tabsGap = MonacoTheme.Space.sm
    static let coverHeight: CGFloat = 136
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
        case .chat: []
        case .treasury: [CabalTreasuryTabSlot.self]
        case .rules: [CabalRulesSlot.self]
        case .members: [CabalMembersSlot.self]
        case .voters: [CabalVotersSlot.self]
        }
    }

    static func body(for context: CabalContext) -> some View {
        CabalTabsLive(context: context)
    }
}

struct CabalTabBar: View {
    @Bindable var selection: CabalTabSelection
    let memberCount: Int32?
    var chatHasUnread = false
    var identifierPrefix = "cabal-tab"

    var body: some View {
        MonacoUnderlineTabs(
            CabalTab.shown, selection: $selection.tab,
            count: { $0.count(memberCount: memberCount) },
            dot: { $0 == .chat && chatHasUnread },
            identifier: { "\(identifierPrefix)-\($0.rawValue)" },
            label: { $0.title }
        )
    }
}

private struct CabalTabsLive: View {
    let context: CabalContext

    @Environment(\.cabalTabSelection) private var shared
    @Environment(\.cabalModel) private var cabalModel
    @Environment(\.cabalChatUnread) private var unread
    @State private var local = CabalTabSelection()

    var body: some View {
        let selection = shared ?? local
        let cabal = cabalModel?.cabal
        let isActiveMember = cabal.map { $0.me != nil && !CabalClosedModel.isReadOnly($0.status) } ?? false
        let chatHasUnread =
            CabalTab.hostsChat && selection.tab != .chat && isActiveMember && (unread?.hasUnread ?? false)
        VStack(alignment: .leading, spacing: CabalPageMetrics.tabsGap) {
            CabalTabBar(selection: selection, memberCount: cabal?.memberCount, chatHasUnread: chatHasUnread)
                .background {
                    GeometryReader { proxy in
                        Color.clear.preference(key: CabalTabsTop.self, value: proxy.frame(in: .global).minY)
                    }
                }
                .accessibilityElement(children: .contain)
                .accessibilityIdentifier("cabal-tabs")
            SectionStackLayout(spacing: CabalPageMetrics.spacing) {
                ForEach(Array(CabalTabs.sections(for: selection.tab).enumerated()), id: \.offset) { _, section in
                    AnyView(section.body(for: context))
                }
            }
            .id(selection.tab)
        }
        .onChange(of: selection.tab) { _, tab in
            if tab == .chat { unread?.markRead() }
        }
    }
}
