import Foundation

extension Components.Schemas.AssetDetail {
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
        quotable: true,
        session: .open,
        decimals: 8,
        uiMultiplier: .init(num: 1, den: 1),
        tradable: true,
        otherListings: [],
        attribution: "Data provided by CoinGecko"
    )

    public static let paused = Self(
        symbol: "SBUXx",
        displayName: "Starbucks xStock",
        issuer: .xstocks,
        kind: .equity,
        logoUrl: nil,
        priceMicros: 98_300_000,
        priceAsOf: Date(timeIntervalSince1970: 1_772_596_200),
        changeBps: -40,
        sparklineMicros: [98_700_000, 98_300_000],
        quotable: false,
        session: .open,
        decimals: 8,
        uiMultiplier: .init(num: 1, den: 1),
        tradable: true,
        otherListings: [],
        attribution: "Data provided by CoinGecko"
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
        quotable: true,
        session: .init(state: .open, continuous: true, holiday: "", earlyClose: false),
        decimals: 6,
        uiMultiplier: .init(num: 1, den: 1),
        tradable: true,
        otherListings: [
            .init(symbol: "SPACEX", displayName: "SpaceX", issuer: .prestocks, kind: .preIpo, tradable: false)
        ],
        attribution: "Data provided by CoinGecko"
    )
}
