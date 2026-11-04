import SwiftUI
import Testing

@testable import Monaco

@MainActor
struct GroupChatRowViewTests {
    @Test func avatarIsTwentyEightPoints() {
        #expect(ChatBubbleMetrics.avatarSize == 28)
    }

    @Test func bubblesUseTheCompactRadius() {
        #expect(MonacoTheme.Radius.compact == 20)
    }

    @Test func mineAndTheirsUseNeutralFills() {
        #expect(GroupChatRowView.bubbleFill(isMine: true) == MonacoTheme.chatBubbleMine)
        #expect(GroupChatRowView.bubbleFill(isMine: false) == MonacoTheme.surfaceSunken)
    }

    @Test func nameShowsAboveTheFirstBubbleOfTheirRuns() {
        #expect(GroupChatRowView.showsAuthorName(startsRun: true, isMine: false))
        #expect(!GroupChatRowView.showsAuthorName(startsRun: false, isMine: false))
        #expect(!GroupChatRowView.showsAuthorName(startsRun: true, isMine: true))
    }

    @Test func faceShowsOnTheLastBubbleOfTheirRuns() {
        #expect(GroupChatRowView.showsFace(endsRun: true, isMine: false))
        #expect(!GroupChatRowView.showsFace(endsRun: false, isMine: false))
        #expect(!GroupChatRowView.showsFace(endsRun: true, isMine: true))
    }
}
