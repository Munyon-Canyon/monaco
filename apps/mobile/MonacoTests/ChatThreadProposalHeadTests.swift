import MonacoAPI
import MonacoCore
import SwiftUI
import Testing

@testable import Monaco

@MainActor
struct ChatThreadProposalHeadTests {
    private static let now = Date(timeIntervalSince1970: 1_791_028_800)

    private static func parent(
        body: String? = "Small add to the position.", proposalID: String? = "p1", deleted: Bool = false,
        replyCount: Int = 6
    ) -> ChatMessage {
        var message = ChatMessage(
            id: "m1", kind: .proposal,
            author: .init(id: "u-mia", handle: nil, displayName: "Mia", photoUrl: nil),
            body: body, createdAt: now.addingTimeInterval(-7_200),
            alsoInChannel: false, replyCount: replyCount, lastRepliers: [], deleted: deleted,
            reactions: [], reactionCount: 0)
        message.proposalId = proposalID
        return message
    }

    private static func quote(
        excerpt: String? = "44% NVDA after this.", deleted: Bool = false, attachment: Bool = false
    ) -> Components.Schemas.CommentQuote {
        Components.Schemas.CommentQuote(
            commentId: "c1", rootCommentId: "c1", authorDisplayName: "Logan", excerpt: excerpt,
            isDeleted: deleted, hasAttachment: attachment, isFolded: false)
    }

    @Test func aProposalParentGetsTheProposalHead() {
        #expect(ChatThreadHead.proposalID(of: Self.parent()) == "p1")
        #expect(ChatThreadHead.proposalID(of: Self.parent(proposalID: nil)) == nil)
        #expect(ChatThreadHead.proposalID(of: Self.parent(deleted: true)) == nil)
        #expect(ChatThreadHead.proposalID(of: nil) == nil)
    }

    @Test func theHeadShowsTheThesisOnlyWhenThereIsOne() {
        #expect(ChatThreadProposalHead.thesis(of: Self.parent()) == "Small add to the position.")
        #expect(ChatThreadProposalHead.thesis(of: Self.parent(body: nil)) == nil)
        #expect(ChatThreadProposalHead.thesis(of: Self.parent(body: "  \n")) == nil)
    }

    @Test func theHeadNamesTheAgeAndTheRule() {
        #expect(ChatThreadProposalHead.ageText(of: Self.parent(), now: Self.now) == "2h")
        #expect(ChatThreadProposalHead.showsRule(of: Self.parent(replyCount: 6)))
        #expect(!ChatThreadProposalHead.showsRule(of: Self.parent(replyCount: 0)))
        #expect(ChatThreadHeadMetrics.faceSize == 40)
    }

    @Test func aQuoteShowsTheNameAndTheExcerpt() {
        let display = ChatReplyQuote.Display(Self.quote())
        #expect(display.name == "Logan")
        #expect(display.excerpt == "44% NVDA after this.")
        #expect(display.targetID == "c1")
        #expect(display.canJump)
    }

    @Test func aDeletedQuoteSaysReplyDeletedAndDoesNotJump() {
        let display = ChatReplyQuote.Display(Self.quote(excerpt: nil, deleted: true))
        #expect(display.excerpt == "Reply deleted")
        #expect(!display.canJump)
    }

    @Test func aGifQuoteSaysGif() {
        #expect(ChatReplyQuote.Display(Self.quote(excerpt: nil, attachment: true)).excerpt == "GIF")
    }

    @Test func aQuoteFindsItsRowByCommentID() {
        var reply = Self.parent(proposalID: nil)
        reply.kind = .user
        reply.commentId = "c1"
        #expect(ChatThreadList.quotedRowID(commentID: "c1", in: [reply]) == "m1")
        #expect(ChatThreadList.quotedRowID(commentID: "c9", in: [reply]) == nil)
    }
}
