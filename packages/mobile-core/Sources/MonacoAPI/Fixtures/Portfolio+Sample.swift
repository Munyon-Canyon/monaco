#if DEBUG
import Foundation

extension Components.Schemas.CabalRef {
    public static let sampleAlpha = Self(
        id: "01890a5d-ac96-774b-bcce-b302099a8058", name: "Alpha", pictureUrl: nil)
    public static let sampleBeta = Self(
        id: "01890a5d-ac96-774b-bcce-b302099a8059", name: "Sunday Investors", pictureUrl: nil)
    public static let sampleACCapital = Self(
        id: "01890a5d-ac96-774b-bcce-b302099a805a", name: "AC Capital", pictureUrl: nil)
    public static let sampleTechTitans = Self(
        id: "01890a5d-ac96-774b-bcce-b302099a805b", name: "Tech Titans", pictureUrl: nil)
    public static let sampleDegens = Self(
        id: "01890a5d-ac96-774b-bcce-b302099a805c", name: "Degens", pictureUrl: nil)
    public static let sampleQuantKids = Self(
        id: "01890a5d-ac96-774b-bcce-b302099a805d", name: "Quant Kids", pictureUrl: nil)
}

extension Components.Schemas.MyPortfolio {
    public static let sample = Self(
        cashMicros: 0, netWorthMicros: 1_000_000_000, totalValueMicros: 1_000_000_000, pnlMicros: 14_000_000,
        returnBps: 142,
        computedAt: Date(timeIntervalSince1970: 1_790_996_460),
        pricesAsOf: Date(timeIntervalSince1970: 1_790_996_400),
        cabals: [
            Components.Schemas.PortfolioCabal(
                cabal: .sampleAlpha, valueMicros: 600_000_000, shareUnits: 600_000_000,
                netContributedMicros: 590_000_000, pnlMicros: 10_000_000, returnBps: 169, sliceBps: 6000,
                ownershipBps: 3500, role: .admin, canVote: true, potValueMicros: 1_714_285_714, memberCount: 8,
                openVoteCount: 1),
            Components.Schemas.PortfolioCabal(
                cabal: .sampleBeta, valueMicros: 400_000_000, shareUnits: 400_000_000,
                netContributedMicros: 396_000_000, pnlMicros: 4_000_000, returnBps: 101, sliceBps: 4000,
                ownershipBps: 1200, role: .member, canVote: true, potValueMicros: 3_333_333_333, memberCount: 6,
                openVoteCount: 0),
        ])

    public static let sampleLoss = Self(
        cashMicros: 0, netWorthMicros: 949_000_000, totalValueMicros: 949_000_000, pnlMicros: -1_000_000,
        returnBps: -11,
        computedAt: Date(timeIntervalSince1970: 1_790_996_460),
        pricesAsOf: Date(timeIntervalSince1970: 1_790_996_400),
        cabals: [
            Components.Schemas.PortfolioCabal(
                cabal: .sampleAlpha, valueMicros: 949_000_000, shareUnits: 949_000_000,
                netContributedMicros: 950_000_000, pnlMicros: -1_000_000, returnBps: -11, sliceBps: 10000,
                ownershipBps: 3500, role: .admin, canVote: true, potValueMicros: 2_711_428_571, memberCount: 8,
                openVoteCount: 0),
            Components.Schemas.PortfolioCabal(
                cabal: .sampleBeta, valueMicros: 0, shareUnits: 0, netContributedMicros: 0, pnlMicros: 0,
                returnBps: nil, sliceBps: 0, ownershipBps: 0, role: .member, canVote: true, potValueMicros: 0,
                memberCount: 6, openVoteCount: 0),
        ])

    public static let sampleOwnProfile = Self(
        cashMicros: 500_000_000, netWorthMicros: 12_082_000_000, totalValueMicros: 11_582_000_000,
        pnlMicros: 454_080_000, returnBps: 408,
        computedAt: Date(timeIntervalSince1970: 1_790_996_460),
        pricesAsOf: Date(timeIntervalSince1970: 1_790_996_400),
        cabals: [
            Components.Schemas.PortfolioCabal(
                cabal: .sampleACCapital, valueMicros: 7_000_000_000, shareUnits: 7_000_000_000,
                netContributedMicros: 6_687_600_000, pnlMicros: 312_400_000, returnBps: 467, sliceBps: 6045,
                ownershipBps: 3500, role: .admin, canVote: true, potValueMicros: 20_000_000_000, memberCount: 8,
                openVoteCount: 1),
            Components.Schemas.PortfolioCabal(
                cabal: .sampleTechTitans, valueMicros: 2_710_000_000, shareUnits: 2_710_000_000,
                netContributedMicros: 2_756_870_000, pnlMicros: -46_870_000, returnBps: -170, sliceBps: 2339,
                ownershipBps: 1200, role: .member, canVote: true, potValueMicros: 22_580_000_000, memberCount: 6,
                openVoteCount: 0),
            Components.Schemas.PortfolioCabal(
                cabal: .sampleDegens, valueMicros: 1_872_000_000, shareUnits: 1_872_000_000,
                netContributedMicros: 1_683_450_000, pnlMicros: 188_550_000, returnBps: 1120, sliceBps: 1616,
                ownershipBps: 800, role: .member, canVote: true, potValueMicros: 23_400_000_000, memberCount: 6,
                openVoteCount: 0),
        ])

    public static let sampleEmpty = Self(
        cashMicros: 0, netWorthMicros: 0, totalValueMicros: 0, pnlMicros: 0, returnBps: nil, computedAt: nil,
        pricesAsOf: nil, cabals: [])

    public static let sampleWithUnfunded = Self(
        cashMicros: 500_000_000, netWorthMicros: 12_082_000_000, totalValueMicros: 11_582_000_000,
        pnlMicros: 454_080_000, returnBps: 408, computedAt: Date(timeIntervalSince1970: 1_790_996_460),
        pricesAsOf: Date(timeIntervalSince1970: 1_790_996_400),
        cabals: sampleOwnProfile.cabals + [
            Components.Schemas.PortfolioCabal(
                cabal: .sampleQuantKids, valueMicros: 0, shareUnits: 0, netContributedMicros: 0, pnlMicros: 0,
                returnBps: nil, sliceBps: 0, ownershipBps: 0, role: .member, canVote: true, potValueMicros: 0,
                memberCount: 5, openVoteCount: 0)
        ])
}

extension Components.Schemas.MyPnlHistory {
    public static func sample(range: RangePayload = ._1d) -> Self {
        Self(
            range: range, pnlMicros: 4_500_000, returnBps: 450,
            points: [
                (100_000_000, 0), (101_500_000, 1_500_000), (103_000_000, 3_000_000), (104_500_000, 4_500_000),
            ].enumerated().map { index, pair in
                Components.Schemas.PnlPoint(
                    at: Date(timeIntervalSince1970: 1_790_931_600 + TimeInterval(index * 3600)),
                    equityMicros: pair.0, pnlMicros: pair.1)
            })
    }

    public static let sampleEmpty = Self(range: ._1d, pnlMicros: 0, returnBps: 0, points: [])

    public static let sampleACCapitalStake = Self(
        range: .all, pnlMicros: 312_400_000, returnBps: 1_249,
        points: [
            (1_785_542_400, 2_500_000_000, 0), (1_788_220_800, 4_100_000_000, 85_000_000),
            (1_790_812_800, 5_600_000_000, 190_000_000), (1_790_996_460, 7_000_000_000, 312_400_000),
        ].map { at, equity, pnl in
            Components.Schemas.PnlPoint(at: Date(timeIntervalSince1970: at), equityMicros: equity, pnlMicros: pnl)
        })

    public static func sampleCurve(range: RangePayload = ._1d, count: Int = 49) -> Self {
        let walk = SampleWalk.equity(count: count, base: 962_000_000, step: 4_000_000, seed: 7)
        let gain = (walk.last ?? walk[0]) - walk[0]
        return Self(
            range: range, pnlMicros: gain, returnBps: gain * 10_000 / walk[0],
            points: walk.enumerated().map { index, equity in
                Components.Schemas.PnlPoint(
                    at: Date(timeIntervalSince1970: 1_790_931_600 + TimeInterval(index * 1_800)),
                    equityMicros: equity, pnlMicros: equity - walk[0])
            })
    }
}

enum SampleWalk {
    static func equity(count: Int, base: Int64, step: Int64, seed: UInt64) -> [Int64] {
        var state = seed &* 0x9E37_79B9_7F4A_7C15
        var value = base
        var out: [Int64] = []
        for index in 0..<count {
            state = state &* 6_364_136_223_846_793_005 &+ 1_442_695_040_888_963_407
            let noise = Int64(state >> 33) % (step * 2) - step + step / 4
            value += noise + (index > count / 2 ? step / 3 : 0)
            out.append(value)
        }
        return out
    }
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

    public static func sampleCurve(
        cabalID: String = Components.Schemas.CabalRef.sampleAlpha.id, range: RangePayload = ._1m, count: Int = 31
    ) -> Self {
        let walk = SampleWalk.equity(count: count, base: 950_000_000, step: 7_000_000, seed: 11)
        return Self(
            cabalId: cabalID, range: range,
            points: walk.enumerated().map { index, value in
                Components.Schemas.CabalValuePoint(
                    at: Date(timeIntervalSince1970: 1_790_931_600 + TimeInterval(index * 86_400)),
                    valueMicros: value, navPerShareMicros: 1_000_000, pnlMicros: value - walk[0])
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
