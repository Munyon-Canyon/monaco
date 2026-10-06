import MonacoAPI

public enum FeedKind: String, CaseIterable, Sendable {
    case proposal
    case trade
    case priceMove = "price_move"
    case cabalCreated = "cabal_created"
    case memberJoined = "member_joined"
}

public enum FeedChip: CaseIterable, Hashable, Sendable {
    case all
    case proposals
    case trades
    case priceMoves
    case cabals

    public var title: String {
        switch self {
        case .all: "All"
        case .proposals: "Proposals"
        case .trades: "Trades"
        case .priceMoves: "Price moves"
        case .cabals: "Cabals"
        }
    }

    var kinds: [FeedKind] {
        switch self {
        case .all: []
        case .proposals: [.proposal]
        case .trades: [.trade]
        case .priceMoves: [.priceMove]
        case .cabals: [.cabalCreated, .memberJoined]
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
    public var chip: FeedChip
    public var scope: FeedScope
    public var search: String

    public init(chip: FeedChip = .all, scope: FeedScope = .everyone, search: String = "") {
        self.chip = chip
        self.scope = scope
        self.search = search
    }

    public var trimmedSearch: String {
        search.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    public func parameters(cursor: String?, limit: Int) -> Operations.GetFeed.Input.Query {
        let kinds = chip.kinds.map(\.rawValue)
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
