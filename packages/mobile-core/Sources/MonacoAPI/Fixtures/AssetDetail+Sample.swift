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
        attribution: "Data provided by CoinGecko",
        priceSource: .jupiter,
        pricePollIntervalSeconds: 120
    )

    public static let nvda = Self(
        symbol: "NVDAx",
        displayName: "NVIDIA xStock",
        issuer: .xstocks,
        kind: .equity,
        logoUrl: "https://cdn.example.com/NVDAx.png",
        priceMicros: 211_170_000,
        priceAsOf: Date(timeIntervalSince1970: 1_772_596_200),
        changeBps: 125,
        sparklineMicros: [208_000_000, 211_170_000],
        volume24hMicros: 9_400_000_000_000,
        quotable: true,
        session: .open,
        decimals: 8,
        uiMultiplier: .init(num: 1, den: 1),
        tradable: true,
        otherListings: [],
        attribution: "Data provided by CoinGecko",
        priceSource: .jupiter,
        pricePollIntervalSeconds: 120,
        stats: .init(
            marketCapMicros: "5380000000000000000",
            openMicros: 211_170_000,
            yearLowMicros: 86_620_000,
            yearHighMicros: 224_900_000,
            heldByCabals: 1
        )
    )

    public static let nvdaDashes = Self(
        symbol: "NVDAx",
        displayName: "NVIDIA xStock",
        issuer: .xstocks,
        kind: .equity,
        logoUrl: "https://cdn.example.com/NVDAx.png",
        priceMicros: 211_170_000,
        priceAsOf: Date(timeIntervalSince1970: 1_772_596_200),
        changeBps: 125,
        sparklineMicros: [208_000_000, 211_170_000],
        quotable: true,
        session: .open,
        decimals: 8,
        uiMultiplier: .init(num: 1, den: 1),
        tradable: true,
        otherListings: [],
        attribution: "Data provided by CoinGecko",
        priceSource: .jupiter,
        pricePollIntervalSeconds: 120,
        stats: .init(
            marketCapMicros: nil,
            openMicros: nil,
            yearLowMicros: nil,
            yearHighMicros: nil,
            heldByCabals: 0
        )
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
        attribution: "Data provided by CoinGecko",
        priceSource: .jupiter,
        pricePollIntervalSeconds: 120
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
        attribution: "Data provided by CoinGecko",
        priceSource: .jupiter,
        pricePollIntervalSeconds: 120,
        preIpo: .init(
            estimate: .init(priceMicros: 185_000_000, asOf: "2026-10-20"),
            valuationMicros: "350000000000000000",
            lastRound: .init(on: "2026-06-01", priceMicros: 165_000_000),
            rounds: [
                .init(on: "2025-02-01", priceMicros: 120_000_000),
                .init(on: "2025-10-01", priceMicros: 150_000_000),
                .init(on: "2026-06-01", priceMicros: 165_000_000),
            ],
            minOrderMicros: 10_000_000,
            sellWindow: .tenderQuarterly,
            nextTenderOn: "2027-01-15",
            imageUrl: nil
        )
    )
}
