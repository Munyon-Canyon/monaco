#if DEBUG
import Foundation

extension Components.Schemas.CabalRef {
    public static let sampleAlpha = Self(
        id: "01890a5d-ac96-774b-bcce-b302099a8058", name: "Alpha", pictureUrl: nil)
    public static let sampleBeta = Self(
        id: "01890a5d-ac96-774b-bcce-b302099a8059", name: "Sunday Investors", pictureUrl: nil)
}

extension Components.Schemas.MyPortfolio {
    public static let sample = Self(
        totalValueMicros: 1_000_000_000, pnlMicros: 14_000_000, returnBps: 142,
        computedAt: Date(timeIntervalSince1970: 1_790_996_460),
        pricesAsOf: Date(timeIntervalSince1970: 1_790_996_400),
        cabals: [
            Components.Schemas.PortfolioCabal(
                cabal: .sampleAlpha, valueMicros: 600_000_000, shareUnits: 600_000_000,
                netContributedMicros: 590_000_000, pnlMicros: 10_000_000, returnBps: 169, sliceBps: 6000),
            Components.Schemas.PortfolioCabal(
                cabal: .sampleBeta, valueMicros: 400_000_000, shareUnits: 400_000_000,
                netContributedMicros: 396_000_000, pnlMicros: 4_000_000, returnBps: 101, sliceBps: 4000),
        ])

    public static let sampleLoss = Self(
        totalValueMicros: 949_000_000, pnlMicros: -1_000_000, returnBps: -11,
        computedAt: Date(timeIntervalSince1970: 1_790_996_460),
        pricesAsOf: Date(timeIntervalSince1970: 1_790_996_400),
        cabals: [
            Components.Schemas.PortfolioCabal(
                cabal: .sampleAlpha, valueMicros: 949_000_000, shareUnits: 949_000_000,
                netContributedMicros: 950_000_000, pnlMicros: -1_000_000, returnBps: -11, sliceBps: 10000),
            Components.Schemas.PortfolioCabal(
                cabal: .sampleBeta, valueMicros: 0, shareUnits: 0, netContributedMicros: 0, pnlMicros: 0,
                returnBps: nil, sliceBps: 0),
        ])

    public static let sampleEmpty = Self(
        totalValueMicros: 0, pnlMicros: 0, returnBps: nil, computedAt: nil, pricesAsOf: nil, cabals: [])
}

extension Components.Schemas.MyPnlHistory {
    public static func sample(range: RangePayload = ._1d) -> Self {
        Self(
            range: range,
            points: [
                (100_000_000, 0), (101_500_000, 1_500_000), (103_000_000, 3_000_000), (104_500_000, 4_500_000),
            ].enumerated().map { index, pair in
                Components.Schemas.PnlPoint(
                    at: Date(timeIntervalSince1970: 1_790_931_600 + TimeInterval(index * 3600)),
                    equityMicros: pair.0, pnlMicros: pair.1)
            })
    }

    public static let sampleEmpty = Self(range: ._1d, points: [])
}

extension Components.Schemas.CabalValueHistory {
    public static func sample(
        cabalID: String = Components.Schemas.CabalRef.sampleAlpha.id, range: RangePayload = ._1m
    ) -> Self {
        Self(
            cabalId: cabalID, range: range,
            points: [
                (500_000_000, 0), (505_000_000, 5_000_000), (498_000_000, -2_000_000), (510_000_000, 10_000_000),
            ].enumerated().map { index, pair in
                Components.Schemas.CabalValuePoint(
                    at: Date(timeIntervalSince1970: 1_790_931_600 + TimeInterval(index * 86_400)),
                    valueMicros: pair.0, navPerShareMicros: 1_000_000, pnlMicros: pair.1)
            },
            pricesAsOf: Date(timeIntervalSince1970: 1_791_190_800))
    }

    public static func sampleShort(cabalID: String = Components.Schemas.CabalRef.sampleBeta.id) -> Self {
        Self(
            cabalId: cabalID, range: ._1m,
            points: [
                Components.Schemas.CabalValuePoint(
                    at: Date(timeIntervalSince1970: 1_790_931_600), valueMicros: 50_000_000,
                    navPerShareMicros: 1_000_000, pnlMicros: 0)
            ],
            pricesAsOf: Date(timeIntervalSince1970: 1_790_931_600))
    }
}

extension Components.Schemas.SharedCabals {
    public static let sample = Self(
        cabals: [
            Components.Schemas.SharedCabal(
                cabal: .sampleAlpha, valueMicros: 950_690_000, pnlMicros: 14_000_000, returnBps: 149),
            Components.Schemas.SharedCabal(
                cabal: .sampleBeta, valueMicros: 120_000_000, pnlMicros: -2_000_000, returnBps: nil),
        ])

    public static let sampleEmpty = Self(cabals: [])
}
#endif
