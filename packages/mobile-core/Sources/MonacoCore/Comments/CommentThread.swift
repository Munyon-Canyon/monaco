import Foundation
import MonacoAPI

extension Components.Schemas.CommentThread: Identifiable {
    public var id: String { comment.id }
}

public struct CommentQuoteRow: Equatable, Sendable {
    public let name: String
    public let excerpt: String
    public let targetID: String
    public let targetRootID: String
    public let targetIsFolded: Bool
    public let isDeleted: Bool

    public init(_ quote: Components.Schemas.CommentQuote) {
        name = quote.authorDisplayName
        targetID = quote.commentId
        targetRootID = quote.rootCommentId
        targetIsFolded = quote.isFolded
        isDeleted = quote.isDeleted
        if quote.isDeleted {
            excerpt = CommentsCopy.quoteDeleted
        } else if let text = quote.excerpt, !text.isEmpty {
            excerpt = text
        } else if quote.hasAttachment {
            excerpt = CommentsCopy.gifTitle
        } else {
            excerpt = CommentsCopy.quoteDeleted
        }
    }

    public var canJump: Bool { !isDeleted }
}

public struct CommentThreadRow: Equatable, Sendable, Identifiable {
    public let comment: Components.Schemas.Comment
    public let isReply: Bool
    public let foldedCount: Int

    public init(comment: Components.Schemas.Comment, isReply: Bool, foldedCount: Int = 0) {
        self.comment = comment
        self.isReply = isReply
        self.foldedCount = foldedCount
    }

    public var id: String { comment.id }

    public var isFolded: Bool { isReply }

    public var indentLevel: Int { isReply ? 1 : 0 }

    public var isDeleted: Bool { comment.body == nil && comment.attachment == nil }

    public var isSystem: Bool { comment.author.kind == .system }

    public var text: String { comment.bodyDisplay }

    public var authorID: String { comment.author.id }

    public var quote: CommentQuoteRow? { isSystem ? nil : comment.replyTo.map(CommentQuoteRow.init) }

    public var reactions: [Components.Schemas.Reaction] { comment.reactions }

    public var attachment: Components.Schemas.CommentAttachment? { comment.attachment }

    public var canReact: Bool { !isSystem && !isDeleted }

    public var authorName: String {
        let name = comment.author.displayName
        if !name.isEmpty { return name }
        return comment.author.handle ?? CommentsCopy.unknownAuthor
    }

    public var opensAuthorProfile: Bool { !isDeleted && !isSystem }

    public var spoken: String {
        let age = RelativeTimeFormatter.label(date: comment.createdAt, now: Date())
        var line = "\(authorName), \(age)"
        if let quote { line += ", replying to \(quote.name)" }
        line += ": \(text)."
        let total = reactions.reduce(0) { $0 + Int($1.count) }
        if total > 0 { line += " \(total) \(total == 1 ? "reaction" : "reactions")" }
        return line
    }

    public static func rows(from threads: [Components.Schemas.CommentThread]) -> [CommentThreadRow] {
        threads.flatMap { thread in
            [CommentThreadRow(comment: thread.comment, isReply: false, foldedCount: thread.replies.count)]
                + thread.replies.map { CommentThreadRow(comment: $0, isReply: true) }
        }
    }
}

public enum CommentThreadEntry: Equatable, Identifiable, Sendable {
    case row(CommentThreadRow)
    case fold(rootID: String, count: Int, isExpanded: Bool)

    public var id: String {
        switch self {
        case .row(let row): row.id
        case .fold(let rootID, _, _): "fold-\(rootID)"
        }
    }
}

public enum ReplyTarget: Equatable, Identifiable {
    case thread
    case comment(CommentThreadRow)
    case post(Components.Schemas.FeedItem)

    public var id: String {
        switch self {
        case .thread: "thread"
        case .comment(let row): "comment-\(row.id)"
        case .post(let item): "post-\(item.id)"
        }
    }

    public var parentCommentID: String? {
        if case .comment(let row) = self { return row.id }
        return nil
    }
}

public enum CommentDraft: Equatable {
    public static let maxScalars = 1000

    case empty
    case tooLong(overBy: Int)
    case ready(body: String)

    public init(text: String) {
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        let count = trimmed.unicodeScalars.count
        if trimmed.isEmpty {
            self = .empty
        } else if count > Self.maxScalars {
            self = .tooLong(overBy: count - Self.maxScalars)
        } else {
            self = .ready(body: trimmed)
        }
    }

    public var body: String? {
        if case .ready(let body) = self { return body }
        return nil
    }
}

extension CommentDraft {
    public static func field(_ field: String, afterPosting body: String) -> String {
        CommentDraft(text: field).body == body ? "" : field
    }
}

public enum CommentsCopy {
    public static let title = "Replies"
    public static let empty = "No replies yet"
    public static let emptyMessage = "Start the discussion."
    public static let placeholder = "Say something"
    public static let membersOnly = "Only members of this cabal can comment."
    public static let deleted = "Comment deleted"
    public static let delete = "Delete"
    public static let reply = "Reply"
    public static let postAccessibility = "Send reply"
    public static let gifAccessibility = "Add a GIF"
    public static let plusAccessibility = "Add a poll, quote or stock"
    public static let gifTitle = "GIF"
    public static let replyTitleBare = "Reply"
    public static let replied = "Replied"
    public static let sending = "Sending…"
    public static let sendFailed = "Your reply wasn’t sent. Your text is still here."
    public static let tooLong = "Comments can be up to 1,000 characters."
    public static let unknownAuthor = "Someone"
    public static let quoteDeleted = "Deleted reply"
    public static let hideReplies = "Hide replies"
    public static let goToQuote = "Go to quoted reply"
    public static let react = "React"

    public static func viewReplies(_ count: Int) -> String {
        count == 1 ? "View 1 reply" : "View \(count) replies"
    }

    public static func replyTitle(_ name: String) -> String {
        "Reply to \(name)"
    }
}
