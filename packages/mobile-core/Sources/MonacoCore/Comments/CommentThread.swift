import MonacoAPI

extension Components.Schemas.CommentThread: Identifiable {
    public var id: String { comment.id }
}

public struct CommentThreadRow: Equatable, Sendable, Identifiable {
    public let comment: Components.Schemas.Comment
    public let isReply: Bool

    public var id: String { comment.id }

    public var indentLevel: Int { isReply ? 1 : 0 }

    public var isDeleted: Bool { comment.body == nil && comment.attachment == nil }

    public var text: String { comment.bodyDisplay }

    public var authorID: String { comment.author.id }

    public var replyToHandle: String? { comment.replyToHandle }

    public var authorName: String {
        let name = comment.author.displayName
        if !name.isEmpty { return name }
        return comment.author.handle ?? CommentsCopy.unknownAuthor
    }

    public var opensAuthorProfile: Bool { !isDeleted }

    public static func rows(from threads: [Components.Schemas.CommentThread]) -> [CommentThreadRow] {
        threads.flatMap { thread in
            [CommentThreadRow(comment: thread.comment, isReply: false)]
                + thread.replies.map { CommentThreadRow(comment: $0, isReply: true) }
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
    public static let title = "Discussion"
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

    public static func replyTitle(_ name: String) -> String {
        "Reply to \(name)"
    }
}
