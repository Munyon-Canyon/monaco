import MonacoAPI

public enum FeedDestination: Equatable, Sendable {
    case proposal(id: String)
    case transaction(cabalID: String, transactionID: String)
    case asset(symbol: String)
    case cabal(id: String)

    public init?(_ item: Components.Schemas.FeedItem) {
        switch FeedKind(rawValue: item.kind) {
        case .proposal:
            self = .proposal(id: item.refId)
        case .trade:
            guard let cabalID = item.cabalId else { return nil }
            self = .transaction(cabalID: cabalID, transactionID: item.refId)
        case .priceMove:
            guard let symbol = item.symbol, !symbol.isEmpty else { return nil }
            self = .asset(symbol: symbol)
        case .cabalCreated, .memberJoined:
            guard let cabalID = item.cabalId else { return nil }
            self = .cabal(id: cabalID)
        case nil:
            return nil
        }
    }
}
