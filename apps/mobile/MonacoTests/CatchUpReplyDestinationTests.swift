import Foundation
import MonacoAPI
import MonacoCore
import Testing

@testable import Monaco

struct CatchUpReplyDestinationTests {
    private let feedItem = "0192e8a1-0005-7000-8000-000000000005"
    private let comment = "0192e8a1-0008-7000-8000-000000000008"
    private let proposal = "0192e8a1-0003-7000-8000-000000000003"
    private let cabal = "0192e8a1-0002-7000-8000-000000000002"
    private let user = "0192e8a1-0001-7000-8000-000000000001"

    @Test func aCatchUpReplyMapsToTheDestinationItsPushDoes() throws {
        let at = Date(timeIntervalSince1970: 1_760_000_000)
        let incoming = Components.Schemas.CatchUpComment(
            feedItemId: feedItem, commentId: comment, parentCommentId: nil, rootCommentId: comment,
            proposalId: proposal, cabalId: cabal, author: .init(id: user, name: "Sam", photoUrl: nil), body: "Hi",
            createdAt: at, reactions: [], reactionCount: 0)
        let item = Components.Schemas.CatchUpItem(
            key: "comment:1", kind: .reply, unread: true, at: at, comment: incoming)
        let push: [String: Any] = [
            "kind": "comment_reply", "feed_item_id": feedItem, "comment_id": comment, "proposal_id": proposal,
            "cabal_id": cabal,
        ]

        let destination = try #require(ReplyDestination(catchUpItem: item))

        #expect(PushRoute.parse(push) == .reply(destination))
    }
}
