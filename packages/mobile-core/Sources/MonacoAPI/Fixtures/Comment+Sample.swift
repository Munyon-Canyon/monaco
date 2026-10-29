#if DEBUG
import Foundation

extension Components.Schemas.Comment {
    public static func sample(
        _ id: String, author: String = "Maya", parent: String? = nil, replyTo: String? = nil,
        quote: ReplyToPayload? = nil, body: String? = nil, mine: Bool = false, minutesAgo: Double = 5
    ) -> Self {
        Self(
            id: id, parentCommentId: parent, rootCommentId: parent,
            author: .init(
                id: "author-\(author.lowercased())", handle: author.lowercased(), displayName: author, photoUrl: nil,
                returnPrivate: false),
            replyToHandle: replyTo, replyTo: quote, body: body ?? "Comment \(id)",
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

extension Components.Schemas.Comment.ReplyToPayload {
    public static func sample(
        _ commentId: String, root: String? = nil, author: String, excerpt: String?, folded: Bool = false
    ) -> Self {
        Self(
            commentId: commentId, rootCommentId: root ?? commentId, authorDisplayName: author, excerpt: excerpt,
            isDeleted: excerpt == nil, isFolded: folded)
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

    public static let aaplThread: [Self] = [
        Self(
            comment: .sample("aapl-nora", author: "Nora", body: "Why sell here?", minutesAgo: 50),
            replies: [
                .sample(
                    "aapl-logan", author: "Logan", parent: "aapl-nora", replyTo: "nora",
                    quote: .sample("aapl-nora", author: "Nora", excerpt: "Why sell here?"),
                    body: "Took the gain while it was there.", minutesAgo: 45)
            ]),
        Self(
            comment: .sample("aapl-andres", author: "Andres", body: "Good timing on this one.", minutesAgo: 40),
            replies: []),
        Self(
            comment: .sample(
                "aapl-christelle", author: "Christelle", replyTo: "logan",
                quote: .sample(
                    "aapl-logan", root: "aapl-nora", author: "Logan", excerpt: "Took the gain while it was there.",
                    folded: true),
                body: "😂", minutesAgo: 30),
            replies: []),
        Self(
            comment: .sample(
                "aapl-sam", author: "Sam", replyTo: "andres",
                quote: .sample("aapl-andres", author: "Andres", excerpt: "Good timing on this one."),
                body: "He called the top again.", minutesAgo: 10),
            replies: []),
    ]

    public static let pilatesThread: [Self] = [
        Self(
            comment: .sample("pilates-zoe", author: "Zoe", body: "Is this too small to matter?", minutesAgo: 25),
            replies: [
                .sample(
                    "pilates-maya", author: "Maya", parent: "pilates-zoe", replyTo: "zoe",
                    quote: .sample("pilates-zoe", author: "Zoe", excerpt: "Is this too small to matter?"),
                    body: "Starter now, more after earnings.", minutesAgo: 20)
            ])
    ]
}
#endif
