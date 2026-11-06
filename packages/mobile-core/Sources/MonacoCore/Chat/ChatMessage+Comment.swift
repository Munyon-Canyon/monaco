import MonacoAPI

public struct ChatReportTarget: Equatable, Sendable {
    public let kind: ReportKind
    public let id: String
}

extension ChatMessage {
    public var commentID: String? { commentId }

    public var isCommentBacked: Bool { commentId != nil }

    public func reactionTarget(cabalID: String) -> ReactionTarget {
        if let commentId { return .comment(commentId) }
        return .chatMessage(cabalID: cabalID, id: id)
    }

    public var reportTarget: ChatReportTarget {
        if let commentId { return ChatReportTarget(kind: .comment, id: commentId) }
        return ChatReportTarget(kind: .message, id: id)
    }

    func matches(id other: String) -> Bool {
        id == other || commentId == other
    }
}
