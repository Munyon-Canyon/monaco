#if DEBUG
import Foundation

extension Components.Schemas.CabalPot {
    public static let sampleInvested = sample(
        potValueMicros: 1_000_000_000, cashMicros: 750_000_000, cashWeightBps: 7500, pnlMicros: 730_000,
        returnBps: 7,
        holdings: [
            Components.Schemas.CabalHolding(
                symbol: "GOOGLx", displayName: "Alphabet", kind: .equity, units: "0.7300", tokenAmount: 73_000_000,
                priceMicros: 342_470_000, valueMicros: 250_000_000, weightBps: 2500, costBasisMicros: 249_270_000,
                pnlMicros: 730_000)
        ],
        me: MePayload(
            shareUnits: 380_000_000, valueMicros: 380_150_000, sliceBps: 3800, netContributedMicros: 380_000_000,
            pnlMicros: 150_000))

    public static let sampleMultichain = sample(
        potValueMicros: 1_547_935_100, cashMicros: 750_000_000, cashWeightBps: 4800, pnlMicros: 730_000,
        returnBps: 7,
        holdings: [
            Components.Schemas.CabalHolding(
                listings: [
                    Components.Schemas.HoldingListing(
                        listingId: "01928a6e-0000-7000-8000-000000000001", issuer: "xstocks", network: .solana,
                        tokenAmount: "73000000", valueMicros: 249_983_100),
                    Components.Schemas.HoldingListing(
                        listingId: "01928a6e-0000-7000-8000-000000000002", issuer: "coinbase", network: .base,
                        tokenAmount: "160000000", valueMicros: 547_952_000),
                ],
                symbol: "GOOGLx", displayName: "Alphabet", kind: .equity, units: "2.3300", tokenAmount: 73_000_000,
                priceMicros: 342_470_000, valueMicros: 797_935_100, weightBps: 5200, costBasisMicros: 797_205_100,
                pnlMicros: 730_000)
        ],
        me: MePayload(
            shareUnits: 380_000_000, valueMicros: 380_150_000, sliceBps: 3800, netContributedMicros: 380_000_000,
            pnlMicros: 150_000))

    public static let sampleACCapital = sample(
        potValueMicros: 20_000_000_000, cashMicros: 5_100_000_000, cashWeightBps: 2550, pnlMicros: 892_330_000,
        returnBps: 467,
        holdings: [
            Components.Schemas.CabalHolding(
                symbol: "NVDAx", displayName: "NVIDIA", kind: .equity, units: "38.0000", tokenAmount: 3_800_000_000,
                priceMicros: 220_000_000, valueMicros: 8_360_000_000, weightBps: 4180, costBasisMicros: 7_332_100_000,
                pnlMicros: 1_027_900_000, avgEntryMicros: "192950000",
                firstBoughtAt: Date(timeIntervalSince1970: 1_780_497_000)),
            Components.Schemas.CabalHolding(
                symbol: "AAPLx", displayName: "Apple", kind: .equity, units: "24.0000", tokenAmount: 2_400_000_000,
                priceMicros: 180_000_000, valueMicros: 4_320_000_000, weightBps: 2160, costBasisMicros: 4_399_200_000,
                pnlMicros: -79_200_000, avgEntryMicros: "183300000",
                firstBoughtAt: Date(timeIntervalSince1970: 1_781_497_000)),
            Components.Schemas.CabalHolding(
                symbol: "tSpaceX", displayName: "SpaceX", kind: .preIpo, units: "12.0000", tokenAmount: 1_200_000_000,
                priceMicros: 185_000_000, valueMicros: 2_220_000_000, weightBps: 1110, costBasisMicros: 2_100_000_000,
                pnlMicros: 120_000_000, avgEntryMicros: "175000000",
                firstBoughtAt: Date(timeIntervalSince1970: 1_783_000_000)),
        ],
        me: MePayload(
            shareUnits: 3_500_000_000, valueMicros: 7_000_000_000, sliceBps: 3500, netContributedMicros: 6_687_600_000,
            pnlMicros: 312_400_000, firstFundedAt: Date(timeIntervalSince1970: 1_785_855_600)))

    public static let sampleCashOnly = sample(
        potValueMicros: 500_000_000, cashMicros: 500_000_000, cashWeightBps: 10000, pnlMicros: 0, holdings: [],
        me: MePayload(
            shareUnits: 500_000_000, valueMicros: 500_000_000, sliceBps: 10000, netContributedMicros: 500_000_000,
            pnlMicros: 0))

    public static let sampleZero = sample(
        potValueMicros: 0, cashMicros: 0, cashWeightBps: 0, pnlMicros: 0, holdings: [],
        me: MePayload(shareUnits: 0, valueMicros: 0, sliceBps: 0, netContributedMicros: 0, pnlMicros: 0))

    public static let sampleOutsider = sample(
        potValueMicros: sampleInvested.potValueMicros, cashMicros: sampleInvested.cashMicros,
        cashWeightBps: sampleInvested.cashWeightBps, pnlMicros: sampleInvested.pnlMicros,
        holdings: sampleInvested.holdings, me: nil)

    public static func sample(
        potValueMicros: Int64, cashMicros: Int64, cashWeightBps: Int32, pnlMicros: Int64, returnBps: Int32? = nil,
        holdings: [Components.Schemas.CabalHolding], me: MePayload?
    ) -> Self {
        Self(
            cabalId: "01890a5d-ac96-774b-bcce-b302099a8059", potValueMicros: potValueMicros, cashMicros: cashMicros,
            cashWeightBps: cashWeightBps, pnlMicros: pnlMicros, returnBps: returnBps,
            pricesAsOf: Date(timeIntervalSince1970: 1_790_996_400), holdings: holdings, me: me)
    }
}
#endif
