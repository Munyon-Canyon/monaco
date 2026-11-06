import MonacoAPI
import MonacoCore
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

    private static let now = Date(timeIntervalSince1970: 1_791_028_800)

    private static func message(replyCount: Int, repliers: [String]) -> ChatMessage {
        ChatMessage(
            id: "m1", kind: .user,
            author: .init(id: "u-mia", handle: nil, displayName: "Mia", photoUrl: nil),
            body: "put it up", createdAt: now.addingTimeInterval(-3_600),
            alsoInChannel: false, replyCount: replyCount,
            lastReplyAt: now.addingTimeInterval(-60),
            lastRepliers: repliers.map { .init(id: "u-\($0)", handle: nil, displayName: $0, photoUrl: nil) },
            deleted: false, reactions: [], reactionCount: 0)
    }

    @Test func theThreadLineShowsAtMostThreeFaces() {
        let message = Self.message(replyCount: 9, repliers: ["Sam", "Kai", "Logan", "Ana"])
        #expect(ChatThreadLine.faces(of: message).map(\.displayName) == ["Sam", "Kai", "Logan"])
    }

    @Test func theThreadLineReadsTheCountTheAgeAndTheNames() {
        let message = Self.message(replyCount: 6, repliers: ["Sam", "Kai", "Logan"])
        #expect(ChatThreadLine.ageText(of: message, now: Self.now) == "1m")
        #expect(
            ChatThreadLine.spokenLabel(of: message, now: Self.now)
                == "6 replies, last 1 minute ago, from Sam, Kai and Logan. Opens the thread.")
    }

    @Test func anOldRowWithNoFacesKeepsTheCountAndAge() {
        let message = Self.message(replyCount: 2, repliers: [])
        #expect(ChatThreadLine.faces(of: message).isEmpty)
        #expect(
            ChatThreadLine.spokenLabel(of: message, now: Self.now)
                == "2 replies, last 1 minute ago. Opens the thread.")
    }

    @Test func aProposalReplyHeaderFallsBackWhileTheCardLoads() {
        #expect(GroupChatRowView.proposalHeaderText(nil) == "replied to a proposal")
        #expect(GroupChatRowView.proposalHeaderText(.loading) == "replied to a proposal")
        #expect(GroupChatRowView.proposalHeaderText(.failed) == "replied to a proposal")
    }

    @Test func aProposalReplyHeaderNamesTheLoadedCard() {
        var proposal = Components.Schemas.Proposal.sample()
        proposal.symbol = "NVDAx"
        proposal.usdcMicros = 400_000_000
        let facts = ChatProposalFacts(summary: ProposalSummary(proposal), asset: nil)
        #expect(GroupChatRowView.proposalHeaderText(.loaded(facts)) == "replied to Buy NVDA \u{00B7} $400")
    }
}
