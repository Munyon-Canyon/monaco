public enum ReactionTarget: Hashable, Sendable {
    case feedItem(String)
    case comment(String)

    public var id: String {
        switch self {
        case .feedItem(let id), .comment(let id): id
        }
    }

    public var key: String {
        switch self {
        case .feedItem(let id): "feed-\(id)"
        case .comment(let id): "comment-\(id)"
        }
    }
}
