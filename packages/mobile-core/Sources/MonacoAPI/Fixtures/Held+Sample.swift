#if DEBUG
import Foundation

extension Components.Schemas.HeldList {
    public static let sampleAndres = Self(
        totalValueMicros: 19_627_710_000, cabalCount: 3,
        pricesAsOf: Date(timeIntervalSince1970: 1_790_996_400),
        holdings: [
            .sample(
                cabalId: "01890a5d-ac96-774b-bcce-b302099a8059", cabalName: "AC Capital",
                logoUrl: "https://cdn.example.com/NVDAx.png",
                holding: Components.Schemas.CabalHolding(
                    symbol: "NVDAx", displayName: "NVIDIA", kind: .equity, units: "38.0000",
                    tokenAmount: 3_800_000_000, priceMicros: 220_000_000, valueMicros: 8_360_000_000,
                    weightBps: 4180, costBasisMicros: 7_332_100_000, pnlMicros: 1_027_900_000,
                    avgEntryMicros: "192950000", firstBoughtAt: Date(timeIntervalSince1970: 1_780_497_000))),
            .sample(
                cabalId: "01890a5d-ac96-774b-bcce-b302099a8059", cabalName: "AC Capital",
                logoUrl: "https://cdn.example.com/AAPLx.png",
                holding: Components.Schemas.CabalHolding(
                    symbol: "AAPLx", displayName: "Apple", kind: .equity, units: "24.0000",
                    tokenAmount: 2_400_000_000, priceMicros: 180_000_000, valueMicros: 4_320_000_000,
                    weightBps: 2160, costBasisMicros: 4_399_200_000, pnlMicros: -79_200_000,
                    avgEntryMicros: "183300000", firstBoughtAt: Date(timeIntervalSince1970: 1_781_000_000))),
            .sample(
                cabalId: "01890a5d-ac96-774b-bcce-b302099a8061", cabalName: "Degens",
                logoUrl: "https://cdn.example.com/METAx.png",
                holding: Components.Schemas.CabalHolding(
                    symbol: "METAx", displayName: "Meta", kind: .equity, units: "4.0000",
                    tokenAmount: 400_000_000, priceMicros: 724_400_000, valueMicros: 2_897_600_000,
                    weightBps: 2415, costBasisMicros: 2_866_000_000, pnlMicros: 31_600_000,
                    avgEntryMicros: "716500000", firstBoughtAt: Date(timeIntervalSince1970: 1_782_000_000))),
            .sample(
                cabalId: "01890a5d-ac96-774b-bcce-b302099a8059", cabalName: "AC Capital",
                logoUrl: "https://cdn.example.com/tSpaceX.png",
                holding: Components.Schemas.CabalHolding(
                    symbol: "tSpaceX", displayName: "SpaceX", kind: .preIpo, units: "12.0000",
                    tokenAmount: 1_200_000_000, priceMicros: 185_000_000, valueMicros: 2_220_000_000,
                    weightBps: 1110, costBasisMicros: 2_100_000_000, pnlMicros: 120_000_000,
                    avgEntryMicros: "175000000", firstBoughtAt: Date(timeIntervalSince1970: 1_783_000_000))),
            .sample(
                cabalId: "01890a5d-ac96-774b-bcce-b302099a8061", cabalName: "Degens",
                logoUrl: "https://cdn.example.com/AMZNx.png",
                holding: Components.Schemas.CabalHolding(
                    symbol: "AMZNx", displayName: "Amazon", kind: .equity, units: "5.0000",
                    tokenAmount: 500_000_000, priceMicros: 184_500_000, valueMicros: 922_500_000,
                    weightBps: 768, costBasisMicros: 880_250_000, pnlMicros: 42_250_000,
                    avgEntryMicros: "176050000", firstBoughtAt: Date(timeIntervalSince1970: 1_784_000_000))),
            .sample(
                cabalId: "01890a5d-ac96-774b-bcce-b302099a8062", cabalName: "Tech Titans",
                logoUrl: "https://cdn.example.com/TSLAx.png",
                holding: Components.Schemas.CabalHolding(
                    symbol: "TSLAx", displayName: "Tesla", kind: .equity, units: "2.2000",
                    tokenAmount: 220_000_000, priceMicros: 412_550_000, valueMicros: 907_610_000,
                    weightBps: 401, costBasisMicros: 648_290_000, pnlMicros: 259_320_000,
                    avgEntryMicros: "294677272", firstBoughtAt: Date(timeIntervalSince1970: 1_785_000_000))),
        ])

    public static let sampleEmpty = Self(
        totalValueMicros: 0, cabalCount: 0, pricesAsOf: Date(timeIntervalSince1970: 1_790_996_400), holdings: [])
}

extension Components.Schemas.HeldRow {
    public static func sample(
        cabalId: String, cabalName: String, logoUrl: String?, holding: Components.Schemas.CabalHolding
    ) -> Self {
        Self(
            cabal: CabalPayload(id: cabalId, name: cabalName, pictureUrl: nil), logoUrl: logoUrl, holding: holding)
    }
}
#endif
