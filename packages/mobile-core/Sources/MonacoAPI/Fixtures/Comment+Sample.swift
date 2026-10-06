#if DEBUG
import Foundation

extension Components.Schemas.Comment {
    public static func sample(
        _ id: String, author: String = "Maya", parent: String? = nil, replyTo: String? = nil, body: String? = nil,
        mine: Bool = false, minutesAgo: Double = 5
    ) -> Self {
        Self(
            id: id, parentCommentId: parent,
            author: .init(
                id: "author-\(author.lowercased())", handle: author.lowercased(), displayName: author, photoUrl: nil),
            replyToHandle: replyTo, body: body ?? "Comment \(id)",
            bodyDisplay: body ?? "Comment \(id)", isMine: mine,
            createdAt: Date(timeIntervalSinceNow: -minutesAgo * 60))
    }

    public static func deletedSample(_ id: String, parent: String? = nil) -> Self {
        var comment = sample(id, parent: parent)
        comment.body = nil
        comment.bodyDisplay = "Comment deleted"
        return comment
    }
}

extension Components.Schemas.CommentThread {
    public static let samples: [Self] = [
        Self(
            comment: .sample("c1", author: "Maya", body: "Earnings beat three quarters running.", minutesAgo: 30),
            replies: [
                .sample(
                    "c2", author: "Alex", parent: "c1", body: "Agreed, but the price already moved.", minutesAgo: 20),
                .sample(
                    "c3", author: "Maya", parent: "c1", replyTo: "alex", body: "Fair. A small slice then.",
                    minutesAgo: 10),
            ]),
        Self(comment: .sample("c4", author: "Sam", body: "In.", minutesAgo: 3), replies: []),
    ]
}
#endif
