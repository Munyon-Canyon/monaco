import MonacoAPI
import SwiftUI
import Testing

@testable import Monaco

@MainActor
struct ChatTapbackBadgeTests {
    private let reactions: [Components.Schemas.Reaction] = [
        .init(emoji: "👍", count: 2, mine: true),
        .init(emoji: "😂", count: 3, mine: false),
        .init(emoji: "🔥", count: 1, mine: false),
        .init(emoji: "🎉", count: 1, mine: false),
    ]

    @Test func summaryShowsThreeEmojiHighestCountFirst() {
        #expect(ChatTapbackBadge.summaryEmoji(reactions) == "😂👍🔥")
    }

    @Test func totalAddsEveryCount() {
        #expect(ChatTapbackBadge.total(reactions) == 7)
    }

    @Test func accessibilityLabelListsCounts() {
        let label = ChatTapbackBadge.accessibilityLabel([
            .init(emoji: "👍", count: 2, mine: false), .init(emoji: "😂", count: 3, mine: false),
        ])
        #expect(label == "5 reactions: 😂 ×3, 👍 ×2")
    }

    @Test func singleReactionIsSingular() {
        #expect(ChatTapbackBadge.accessibilityLabel([.init(emoji: "😂", count: 1, mine: false)]) == "1 reaction: 😂 ×1")
    }

    @Test func tapRemovesMyReactionElseOpensTray() {
        #expect(ChatTapbackBadge.tapAction(reactions) == .remove("👍"))
        #expect(ChatTapbackBadge.tapAction([.init(emoji: "😂", count: 3, mine: false)]) == .openTray)
    }
}
