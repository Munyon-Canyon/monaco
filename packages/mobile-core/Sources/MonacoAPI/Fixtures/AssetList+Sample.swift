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

    public static let moversSample = Self(
        assets: [.amd, .nflx, .tsla, .amzn, .msft],
        nextCursor: nil
    )

    public static let allSample = Self(
        assets: [.googl, .unpriced],
        nextCursor: "all-next"
    )
}

extension Components.Schemas.AssetSummary {
    private static func mover(
        symbol: String, displayName: String, changeBps: Int32, sparklineMicros: [Int64]
    ) -> Self {
        Self(
            symbol: symbol,
            displayName: displayName,
            issuer: .xstocks,
            kind: .equity,
            logoUrl: nil,
            priceMicros: sparklineMicros.last,
            priceAsOf: Date(timeIntervalSince1970: 1_772_596_200),
            changeBps: changeBps,
            sparklineMicros: sparklineMicros,
            quotable: true,
            session: .open,
            tradable: true
        )
    }

    public static let amd = mover(
        symbol: "AMDx", displayName: "Advanced Micro Devices xStock", changeBps: 790,
        sparklineMicros: [150_000_000, 153_000_000, 158_000_000, 161_850_000]
    )
    public static let nflx = mover(
        symbol: "NFLXx", displayName: "Netflix xStock", changeBps: 520,
        sparklineMicros: [900_000_000, 915_000_000, 930_000_000, 946_800_000]
    )
    public static let tsla = mover(
        symbol: "TSLAx", displayName: "Tesla xStock", changeBps: -240,
        sparklineMicros: [250_000_000, 247_000_000, 246_000_000, 244_000_000]
    )
    public static let amzn = mover(
        symbol: "AMZNx", displayName: "Amazon xStock", changeBps: -210,
        sparklineMicros: [200_000_000, 198_500_000, 197_000_000, 195_800_000]
    )
    public static let msft = mover(
        symbol: "MSFTx", displayName: "Microsoft xStock", changeBps: 60,
        sparklineMicros: [420_000_000, 421_000_000, 422_000_000, 422_520_000]
    )

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
        tradable: true
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
        tradable: true
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
        quotable: true,
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
        quotable: true,
        session: .init(state: .open, continuous: true, holiday: "", earlyClose: false),
        tradable: true
    )
}

extension Components.Schemas.MarketSession {
    public static let preMarket = Self(state: .preMarket, continuous: false, holiday: "", earlyClose: false)
    public static let open = Self(state: .open, continuous: false, holiday: "", earlyClose: false)
    public static let afterHours = Self(state: .afterHours, continuous: false, holiday: "", earlyClose: false)
    public static let closed = Self(state: .closed, continuous: false, holiday: "", earlyClose: false)
}
