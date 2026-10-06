import MonacoAPI

public struct FeedMuteTarget: Hashable, Sendable {
    public enum Kind: String, Sendable {
        case kind, cabal, asset, user, item
    }

    public let kind: Kind
    public let id: String

    public init(_ kind: Kind, _ id: String) {
        self.kind = kind
        self.id = id
    }

    public init?(_ mute: Components.Schemas.FeedMute) {
        guard let kind = Kind(rawValue: mute.targetType) else { return nil }
        self.init(kind, mute.targetId)
    }
}

extension Components.Schemas.FeedMute: Identifiable {
    public var id: String { "\(targetType)/\(targetId)" }
}

public enum FeedMuteMatcher {
    public static func removes(_ target: FeedMuteTarget, _ item: Components.Schemas.FeedItem) -> Bool {
        switch target.kind {
        case .kind: item.kind == target.id
        case .cabal: same(item.cabalId, target.id)
        case .asset: same(item.assetId, target.id)
        case .user: same(item.actorId, target.id)
        case .item: same(item.id, target.id)
        }
    }

    private static func same(_ id: String?, _ other: String) -> Bool {
        id?.caseInsensitiveCompare(other) == .orderedSame
    }
}

public struct FeedMuteOption: Equatable, Sendable, Identifiable {
    public let target: FeedMuteTarget
    public let menuTitle: String
    public let toast: String

    public var id: FeedMuteTarget { target }

    public static func hide(_ item: Components.Schemas.FeedItem) -> Self {
        Self(target: .init(.item, item.id), menuTitle: "Hide this post", toast: "Hidden.")
    }

    static func mute(_ target: FeedMuteTarget, label: String) -> Self {
        Self(target: target, menuTitle: "Mute \(label)", toast: "Muted \(label).")
    }

    public static func options(for item: Components.Schemas.FeedItem, viewerID: String?) -> [Self] {
        var options = [hide(item)]
        if let cabalID = item.cabalId, let name = item.cabalName, !name.isEmpty {
            options.append(.mute(.init(.cabal, cabalID), label: name))
        }
        if let assetID = item.assetId, let symbol = item.symbol, !symbol.isEmpty {
            options.append(.mute(.init(.asset, assetID), label: symbol))
        }
        if let actorID = item.actorId, let name = item.actorName, !name.isEmpty,
            actorID.caseInsensitiveCompare(viewerID ?? "") != .orderedSame
        {
            options.append(.mute(.init(.user, actorID), label: name))
        }
        if let kind = FeedKind(rawValue: item.kind) {
            options.append(.mute(.init(.kind, kind.rawValue), label: kind.muteLabel))
        }
        return options
    }
}

extension FeedKind {
    var muteLabel: String {
        switch self {
        case .proposal: "Proposals"
        case .trade: "Trades"
        case .priceMove: "Price moves"
        case .cabalCreated: "New cabals"
        case .memberJoined: "New members"
        }
    }
}
