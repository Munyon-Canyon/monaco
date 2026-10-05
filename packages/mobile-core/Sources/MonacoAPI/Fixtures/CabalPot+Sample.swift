#if DEBUG
import Foundation

extension Components.Schemas.CabalPot {
    public static let sampleInvested = sample(
        potValueMicros: 1_000_000_000, cashMicros: 750_000_000, cashWeightBps: 7500, pnlMicros: 730_000,
        holdings: [
            Components.Schemas.CabalHolding(
                symbol: "GOOGLx", displayName: "Alphabet", units: "0.7300", priceMicros: 342_470_000,
                valueMicros: 250_000_000, weightBps: 2500, costBasisMicros: 249_270_000, pnlMicros: 730_000)
        ],
        me: MePayload(
            shareUnits: 380_000_000, valueMicros: 380_150_000, sliceBps: 3800, netContributedMicros: 380_000_000,
            pnlMicros: 150_000))

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
        potValueMicros: Int64, cashMicros: Int64, cashWeightBps: Int32, pnlMicros: Int64,
        holdings: [Components.Schemas.CabalHolding], me: MePayload?
    ) -> Self {
        Self(
            cabalId: "01890a5d-ac96-774b-bcce-b302099a8059", potValueMicros: potValueMicros, cashMicros: cashMicros,
            cashWeightBps: cashWeightBps, pnlMicros: pnlMicros, returnBps: nil,
            pricesAsOf: Date(timeIntervalSince1970: 1_790_996_400), holdings: holdings, me: me)
    }
}
#endif
