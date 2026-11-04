public enum ReactionTarget: Hashable, Sendable {
    case feedItem(String)
    case comment(String)
    case chatMessage(cabalID: String, id: String)

    public var id: String {
        switch self {
        case .feedItem(let id), .comment(let id), .chatMessage(_, let id): id
        }
    }

    public var key: String {
        switch self {
        case .feedItem(let id): "feed-\(id)"
        case .comment(let id): "comment-\(id)"
        case .chatMessage(_, let id): "chat-\(id)"
        }
    }
}
