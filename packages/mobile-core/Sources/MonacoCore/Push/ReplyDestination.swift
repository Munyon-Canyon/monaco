import Foundation
import MonacoAPI

public struct ReplyDestination: Equatable, Hashable, Sendable {
    public let feedItemID: String
    public let commentID: String
    public let proposalID: String?
    public let cabalID: String?

    public init(feedItemID: String, commentID: String, proposalID: String?, cabalID: String?) {
        self.feedItemID = feedItemID
        self.commentID = commentID
        self.proposalID = proposalID
        self.cabalID = cabalID
    }

    public init?(catchUpItem item: Components.Schemas.CatchUpItem) {
        guard item.kind == .reply, let comment = item.comment else { return nil }
        self.init(
            feedItemID: comment.feedItemId.lowercased(),
            commentID: comment.commentId.lowercased(),
            proposalID: comment.proposalId?.lowercased(),
            cabalID: comment.cabalId?.lowercased())
    }

    public var route: PushRoute { .reply(self) }
}
