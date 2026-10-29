import MonacoAPI
import MonacoCore
import Testing

@testable import Monaco

@MainActor
struct FeedReplyControlTests {
    private func item(
        kind: String, replies: Int32, canReply: Bool = true,
        shape: Components.Schemas.FeedItem.ShapePayload? = nil
    ) -> Components.Schemas.FeedItem {
        var item = Components.Schemas.FeedItem.samples[0]
        item.kind = kind
        item.replyCount = replies
        item.canReply = canReply
        item.refId = "ref-1"
        item.proposalId = "prop-1"
        item.shape = shape
        return item
    }

    @Test func aReplyPostDrawsTheBareBubbleEvenWhenTheThreadHasReplies() {
        #expect(ReplyControlAction.shownCount(for: item(kind: "reply", replies: 4)) == 0)
    }

    @Test func otherPostsShowTheThreadCountAndZeroStaysHidden() {
        #expect(ReplyControlAction.shownCount(for: item(kind: "proposal", replies: 6)) == 6)
        #expect(ReplyControlAction.shownCount(for: item(kind: "trade", replies: 0)) == 0)
    }

    @Test func aProposalOpensItsRepliesAndAReplyPostOpensAtTheReply() {
        #expect(
            ReplyControlAction.action(for: item(kind: "proposal", replies: 6))
                == .openProposal(proposalID: "prop-1", focus: .replies))
        #expect(
            ReplyControlAction.action(for: item(kind: "reply", replies: 4))
                == .openProposal(proposalID: "prop-1", focus: .reply(commentID: "ref-1")))
    }

    @Test func aPostPresentsTheSheetWhenItCanBeRepliedToAndOtherwiseOpensThePost() {
        let post = item(kind: "trade", replies: 1)
        #expect(ReplyControlAction.action(for: post) == .presentSheet)
        let locked = item(kind: "trade", replies: 1, canReply: false)
        #expect(ReplyControlAction.action(for: locked) == .openPost(itemID: locked.id))
    }
}
