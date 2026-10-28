import MonacoAPI

public enum FeedKind: String, CaseIterable, Sendable {
    case proposal
    case trade
    case priceMove = "price_move"
    case cabalCreated = "cabal_created"
    case memberJoined = "member_joined"
    case reply
    case threshold
}

public enum FeedScope: CaseIterable, Hashable, Sendable {
    case network
    case mine
    case following
    case everyone
    case others

    public static let menu: [FeedScope] = [.network, .mine, .following, .everyone]

    public var title: String {
        switch self {
        case .network: "Your cabals and friends"
        case .mine: "Your cabals"
        case .following: "Friends"
        case .everyone: "Everyone"
        case .others: "Other cabals"
        }
    }
}

extension FeedScope {
    var wire: Operations.GetFeed.Input.Query.ScopePayload {
        switch self {
        case .network: .network
        case .mine: .mine
        case .following: .following
        case .everyone: .all
        case .others: .others
        }
    }
}

public struct FeedQuery: Equatable, Sendable {
    public static let shownKinds = "proposal,result,reply,member_joined,cabal_created,trade,threshold"

    public var scope: FeedScope

    public init(scope: FeedScope = .network) {
        self.scope = scope
    }

    public func parameters(cursor: String?, limit: Int) -> Operations.GetFeed.Input.Query {
        .init(kind: Self.shownKinds, scope: scope.wire, cursor: cursor, limit: limit)
    }
}
