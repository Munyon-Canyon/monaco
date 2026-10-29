import MonacoCore
import SwiftUI

enum CabalsVoteSlot: CabalsTabSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        CabalsVoteSection()
    }
}

struct CabalsVoteSection: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(CabalsTabStore.self) private var store
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            if let card = store.votes.shown {
                HomeVoteCard(model: card, identifierPrefix: "cabals", tab: .cabals).id(card.id)
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                    .padding(.bottom, MonacoTheme.Space.m)
            }
        }
        .animation(reduceMotion ? nil : .snappy, value: store.votes.cards.map(\.id))
        .onScreenVisibilityChange { store.votes.setVisible($0) }
        .onChange(of: signature, initial: true) { _, _ in
            store.syncVotes()
            environment.navigator.tabBadges[.cabals] = store.totalWaiting
        }
    }

    private var signature: [String] {
        store.votes.cards.map { "\($0.id)|\($0.isCommitted)" }
    }
}
