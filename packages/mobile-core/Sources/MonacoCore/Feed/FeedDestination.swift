import MonacoAPI

public enum FeedDestination: Equatable, Sendable {
    case proposal(proposalID: String)
    case post(itemID: String)

    public static func destination(for item: Components.Schemas.FeedItem) -> FeedDestination {
        switch FeedPostKind(item.kind) {
        case .proposal, .reply:
            return proposal(for: item)
        case .result:
            return item.shape == .cabal ? proposal(for: item) : .post(itemID: item.id)
        case .trade, .threshold, .memberJoined, .cabalCreated, .other:
            return .post(itemID: item.id)
        }
    }

    private static func proposal(for item: Components.Schemas.FeedItem) -> FeedDestination {
        let fallback = FeedPostKind(item.kind) == .proposal ? item.refId : nil
        guard let proposalID = item.proposalId ?? fallback else { return .post(itemID: item.id) }
        return .proposal(proposalID: proposalID)
    }
}

enum FeedPostKind: Equatable {
    case proposal, reply, result, trade, threshold, memberJoined, cabalCreated, other

    init(_ raw: String) {
        switch raw {
        case "proposal": self = .proposal
        case "reply": self = .reply
        case "result": self = .result
        case "trade": self = .trade
        case "threshold": self = .threshold
        case "member_joined": self = .memberJoined
        case "cabal_created": self = .cabalCreated
        default: self = .other
        }
    }
}
