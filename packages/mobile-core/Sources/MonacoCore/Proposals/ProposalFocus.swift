public enum ProposalFocus: Hashable, Sendable {
    case none
    case replies
    case reply(commentID: String)
    case anchor(commentID: String)

    public static let repliesAnchorID = "proposal-replies"
    public static let maxPages = 10

    public enum Resolution: Equatable {
        case nothing
        case replySheet
        case replySheetQuoting(CommentThreadRow)
        case scroll(commentID: String)
        case needsMore
        case notFound
    }

    public func resolve(in rows: [CommentThreadRow], hasMore: Bool) -> Resolution {
        switch self {
        case .none: return .nothing
        case .replies: return .replySheet
        case .reply(let id):
            if let row = rows.first(where: { $0.id == id }) { return .replySheetQuoting(row) }
            return hasMore ? .needsMore : .notFound
        case .anchor(let id):
            if rows.contains(where: { $0.id == id }) { return .scroll(commentID: id) }
            return hasMore ? .needsMore : .notFound
        }
    }
}
