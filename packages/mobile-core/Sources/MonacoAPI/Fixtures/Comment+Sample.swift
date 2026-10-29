#if DEBUG
import Foundation

extension Components.Schemas.Comment {
    public static func sample(
        _ id: String, author: String = "Maya", parent: String? = nil, replyTo: String? = nil, body: String? = nil,
        mine: Bool = false, minutesAgo: Double = 5, quote: Components.Schemas.CommentQuote? = nil,
        reactions: [Components.Schemas.Reaction] = []
    ) -> Self {
        Self(
            id: id, parentCommentId: parent, rootCommentId: parent,
            author: .init(
                kind: .user, id: "author-\(author.lowercased())", handle: author.lowercased(),
                displayName: author, photoUrl: nil, returnPrivate: false),
            replyToHandle: replyTo, replyTo: quote, body: body ?? "Comment \(id)",
            bodyDisplay: body ?? "Comment \(id)", attachment: nil, isMine: mine,
            reactions: reactions, reactionCount: Int32(reactions.reduce(0) { $0 + Int($1.count) }),
            createdAt: Date(timeIntervalSinceNow: -minutesAgo * 60))
    }

    public static func monacoSample(_ id: String, body: String, at: Date) -> Self {
        Self(
            id: id,
            author: .init(
                kind: .system, id: "00000000-0000-0000-0000-000000000000", handle: nil,
                displayName: "Monaco", photoUrl: nil, returnPrivate: false),
            replyToHandle: nil, body: body, bodyDisplay: body, isMine: false, reactions: [], reactionCount: 0,
            createdAt: at)
    }

    public static func gifSample(
        _ id: String, author: String, parent: String? = nil, minutesAgo: Double = 5,
        quote: Components.Schemas.CommentQuote? = nil, reactions: [Components.Schemas.Reaction] = []
    ) -> Self {
        var comment = sample(
            id, author: author, parent: parent, minutesAgo: minutesAgo, quote: quote, reactions: reactions)
        comment.body = nil
        comment.bodyDisplay = "GIF"
        comment.attachment = .gif(.sample())
        return comment
    }

    public static func thesisQuoteSample(
        _ id: String, author: String, text: String, minutesAgo: Double = 5
    ) -> Self {
        var comment = sample(id, author: author, minutesAgo: minutesAgo)
        comment.body = nil
        comment.bodyDisplay = "Quoted the thesis"
        comment.attachment = .thesisQuote(
            .init(kind: .thesisQuote, proposalId: "proposal-nvda", text: text, proposerName: "Maya"))
        return comment
    }

    public static func stockSample(
        _ id: String, author: String, minutesAgo: Double = 5
    ) -> Self {
        var comment = sample(id, author: author, minutesAgo: minutesAgo)
        comment.body = nil
        comment.bodyDisplay = "Shared a stock"
        comment.attachment = .stock(
            .init(
                kind: .stock, assetId: "asset-aapl", symbol: "AAPL", name: "Apple",
                logoUrl: "file:///placeholder.png", isPreIpo: false, priceMicros: 180_000_000, changeBps: -80))
        return comment
    }

    public static func pollSample(
        _ id: String, author: String, parent: String? = nil, minutesAgo: Double = 5,
        quote: Components.Schemas.CommentQuote? = nil
    ) -> Self {
        var comment = sample(id, author: author, parent: parent, minutesAgo: minutesAgo, quote: quote)
        comment.body = nil
        comment.bodyDisplay = "Trim to 40% before earnings?"
        comment.attachment = .poll(.sample())
        return comment
    }

    public static let priyaPoll = pollSample("poll-priya", author: "Priya", minutesAgo: 10)

    public static func deletedSample(_ id: String, parent: String? = nil) -> Self {
        var comment = sample(id, parent: parent)
        comment.body = nil
        comment.bodyDisplay = "Comment deleted"
        return comment
    }
}

extension Components.Schemas.CommentQuote {
    public static func sample(
        _ commentId: String, root: String? = nil, author: String, excerpt: String?, folded: Bool = false,
        deleted: Bool = false, hasAttachment: Bool = false
    ) -> Self {
        Self(
            commentId: commentId, rootCommentId: root ?? commentId, authorDisplayName: author, excerpt: excerpt,
            isDeleted: deleted, hasAttachment: hasAttachment, isFolded: folded)
    }
}

extension Components.Schemas.GifAttachment {
    public static func sample(id: String = "xT9IgDEI1iZyb2wqo8", title: String = "Clapping") -> Self {
        let media = Components.Schemas.GifMedia(url: "file:///placeholder.gif", width: 200, height: 150)
        return Self(kind: .gif, id: id, title: title, still: media, preview: media, original: media)
    }
}

extension Components.Schemas.PollAttachment {
    public static func sample(
        question: String = "Trim to 40% before earnings?", closesInHours: Double = 20, myPosition: Int? = 1
    ) -> Self {
        Self(
            kind: .poll, question: question, closesAt: Date(timeIntervalSinceNow: closesInHours * 3600), closed: false,
            options: [
                .init(position: 0, label: "Yes", count: 3), .init(position: 1, label: "Wait", count: 2),
            ], total: 5, myPosition: myPosition)
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

    public static let loganAAPLThread: [Self] = [
        Self(
            comment: .sample(
                "aapl-nora", author: "Nora", body: "Why sell here?", minutesAgo: 50,
                reactions: [.init(emoji: "❤️", count: 1, mine: false)]),
            replies: [
                .sample(
                    "aapl-logan", author: "Logan", parent: "aapl-nora", body: "Took the gain while it was there.",
                    mine: true, minutesAgo: 45,
                    quote: .sample("aapl-nora", author: "Nora", excerpt: "Why sell here?"))
            ]),
        Self(
            comment: .sample(
                "aapl-andres", author: "Andres", body: "Good timing on this one.", minutesAgo: 40,
                reactions: [.init(emoji: "❤️", count: 2, mine: false)]),
            replies: []),
        Self(
            comment: .gifSample(
                "aapl-christelle", author: "Christelle", minutesAgo: 30,
                quote: .sample(
                    "aapl-logan", root: "aapl-nora", author: "Logan", excerpt: "Took the gain while it was there.",
                    folded: true),
                reactions: [.init(emoji: "😂", count: 3, mine: false), .init(emoji: "❤️", count: 1, mine: false)]),
            replies: []),
        Self(
            comment: .sample(
                "aapl-jordan", author: "Jordan", body: "Same.", minutesAgo: 25,
                quote: .sample(
                    "aapl-christelle", author: "Christelle", excerpt: "GIF", hasAttachment: true)),
            replies: []),
        Self(
            comment: .sample(
                "aapl-sam", author: "Sam", body: "He called the top again.", minutesAgo: 20,
                quote: .sample("aapl-andres", author: "Andres", excerpt: "Good timing on this one.")),
            replies: []),
    ]

    public static let nvdaThread: [Self] = [
        Self(
            comment: .thesisQuoteSample(
                "nvda-christelle", author: "Christelle",
                text: "Small add to the position. Earnings next week; I want more before, and we can add on a dip.",
                minutesAgo: 20),
            replies: []),
        Self(comment: .stockSample("nvda-jordan", author: "Jordan", minutesAgo: 10), replies: []),
    ]

    public static let nvdaPassedMonacoReply: Self = Self(
        comment: .monacoSample(
            "nvda-monaco", body: "Passed and filled. 38 → 39.82 sh NVDA in the treasury.",
            at: Date(timeIntervalSince1970: 1_792_662_060)),
        replies: [])

    public static let tslaFailedMonacoReply: Self = Self(
        comment: .monacoSample(
            "tsla-monaco", body: "Closed without passing. Nothing was sold.",
            at: Date(timeIntervalSince1970: 1_792_346_400)),
        replies: [])

    public static let pilatesThread: [Self] = [
        Self(
            comment: .sample(
                "pilates-zoe", author: "Zoe", body: "Is this the same studio as last quarter?", minutesAgo: 60),
            replies: [
                .sample(
                    "pilates-maya", author: "Maya", parent: "pilates-zoe", body: "Starter now, more after earnings.",
                    minutesAgo: 55,
                    quote: .sample("pilates-zoe", author: "Zoe", excerpt: "Is this the same studio as last quarter?"))
            ])
    ]
}
#endif
