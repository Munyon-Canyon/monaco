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

public enum FeedKindGroup: CaseIterable, Hashable, Sendable {
    case proposals
    case completedTrades
    case priceMoves
    case repliesAndVotes
    case proposalOutcomes
    case cabalActivity

    public var title: String {
        switch self {
        case .proposals: "Proposals"
        case .completedTrades: "Completed trades"
        case .priceMoves: "Price moves"
        case .repliesAndVotes: "Replies & votes"
        case .proposalOutcomes: "Proposal outcomes"
        case .cabalActivity: "Cabal activity"
        }
    }

    var kinds: [FeedKind] {
        switch self {
        case .proposals: [.proposal]
        case .completedTrades: [.trade]
        case .priceMoves: [.priceMove]
        case .repliesAndVotes: [.reply]
        case .proposalOutcomes: [.threshold]
        case .cabalActivity: [.cabalCreated, .memberJoined]
        }
    }
}

public enum FeedScope: CaseIterable, Hashable, Sendable {
    case everyone
    case mine
    case following

    public var title: String {
        switch self {
        case .everyone: "Everyone"
        case .mine: "My cabals"
        case .following: "Following"
        }
    }
}

extension FeedScope {
    var wire: Operations.GetFeed.Input.Query.ScopePayload {
        switch self {
        case .everyone: .all
        case .mine: .mine
        case .following: .following
        }
    }
}

public struct FeedQuery: Equatable, Sendable {
    public var kinds: Set<FeedKindGroup>
    public var scope: FeedScope
    public var search: String

    public init(kinds: Set<FeedKindGroup> = [], scope: FeedScope = .everyone, search: String = "") {
        self.kinds = kinds
        self.scope = scope
        self.search = search
    }

    public var trimmedSearch: String {
        search.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    public func parameters(cursor: String?, limit: Int) -> Operations.GetFeed.Input.Query {
        let kinds = Set(self.kinds.flatMap(\.kinds)).map(\.rawValue).sorted()
        let q = trimmedSearch
        return .init(
            kind: kinds.isEmpty ? nil : kinds.joined(separator: ","),
            q: q.isEmpty ? nil : q,
            scope: scope.wire,
            cursor: cursor,
            limit: limit
        )
    }
}
