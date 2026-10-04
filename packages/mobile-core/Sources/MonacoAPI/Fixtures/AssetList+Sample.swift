import Foundation

extension Components.Schemas.AssetList {
    public static let popularSample = Self(
        assets: [.googl],
        nextCursor: "popular-next"
    )

    public static let preIpoSample = Self(
        assets: [.spaceX],
        nextCursor: nil
    )

    public static let allSample = Self(
        assets: [.googl, .unpriced],
        nextCursor: "all-next"
    )
}

extension Components.Schemas.AssetSummary {
    public static let googl = Self(
        symbol: "GOOGLx",
        displayName: "Alphabet xStock",
        issuer: .xstocks,
        kind: .equity,
        logoUrl: "https://cdn.example.com/GOOGLx.png",
        priceMicros: 175_420_000,
        priceAsOf: Date(timeIntervalSince1970: 1_772_596_200),
        changeBps: 125,
        sparklineMicros: [174_000_000, 175_420_000],
        session: .open
    )

    public static let unpriced = Self(
        symbol: "NEWCO",
        displayName: "Newco",
        issuer: .xstocks,
        kind: .equity,
        logoUrl: nil,
        priceMicros: nil,
        priceAsOf: nil,
        changeBps: nil,
        sparklineMicros: nil,
        session: .closed
    )

    public static let spaceX = Self(
        symbol: "tSpaceX",
        displayName: "T-SpaceX",
        issuer: .tessera,
        kind: .preIpo,
        logoUrl: nil,
        priceMicros: 42_000_000,
        priceAsOf: Date(timeIntervalSince1970: 1_772_596_200),
        changeBps: 125,
        sparklineMicros: [40_000_000, 42_000_000],
        session: .init(state: .open, continuous: true, holiday: "", earlyClose: false)
    )
}

extension Components.Schemas.MarketSession {
    public static let preMarket = Self(state: .preMarket, continuous: false, holiday: "", earlyClose: false)
    public static let open = Self(state: .open, continuous: false, holiday: "", earlyClose: false)
    public static let afterHours = Self(state: .afterHours, continuous: false, holiday: "", earlyClose: false)
    public static let closed = Self(state: .closed, continuous: false, holiday: "", earlyClose: false)
}
