#if DEBUG
import Foundation

extension Components.Schemas.CabalActivity {
    public static let sampleActor = ActorPayload(
        userId: "01890a5d-ac96-774b-bcce-b302099a8059", handle: "ana", displayName: "Ana")

    public static let sampleBuy = sample(
        1, kind: .buy, status: .confirmed, asset: .init(symbol: "AAPLx", name: "Apple"), micros: 25_000_000,
        actor: nil)
    public static let sampleSell = sample(
        2, kind: .sell, status: .pending, asset: .init(symbol: "TSLAx", name: "Tesla"), micros: nil, actor: nil)
    public static let sampleFund = sample(
        3, kind: .fund, status: .confirmed, asset: nil, micros: 100_000_000, actor: sampleActor)
    public static let sampleCashOut = sample(
        4, kind: .cashOut, status: .failed, asset: nil, micros: 12_500_000, actor: sampleActor)

    public static let sampleWindDownCashOut = sample(
        5, kind: .cashOut, status: .confirmed, asset: nil, micros: 42_000_000, actor: nil)

    public static let samples = [sampleBuy, sampleSell, sampleFund, sampleCashOut]

    public static let sampleNVDATrades = [
        nvdaTrade(9, kind: .buy, shares: 13, priceMicros: 196_750_000, at: 1_790_778_600),
        nvdaTrade(8, kind: .sell, shares: 10, priceMicros: 214_100_000, at: 1_787_236_200),
        nvdaTrade(7, kind: .buy, shares: 15, priceMicros: 188_400_000, at: 1_784_125_800),
        nvdaTrade(6, kind: .buy, shares: 20, priceMicros: 181_200_000, at: 1_780_497_000),
    ]

    static func nvdaTrade(
        _ number: Int, kind: KindPayload, shares: Int64, priceMicros: Int64, at seconds: TimeInterval
    ) -> Self {
        Self(
            id: String(format: "01890a5d-ac96-774b-bcce-b302099a9%03d", number), kind: kind, status: .confirmed,
            asset: .init(symbol: "NVDAx", name: "NVIDIA"), usdcMicros: shares * priceMicros,
            units: shares * 100_000_000, priceMicros: priceMicros, actor: nil,
            txSignature: "sample-signature-\(number)", occurredAt: Date(timeIntervalSince1970: seconds))
    }

    public static func sample(
        _ number: Int, kind: KindPayload, status: StatusPayload, asset: AssetPayload?, micros: Int64?,
        actor: ActorPayload?
    ) -> Self {
        Self(
            id: String(format: "01890a5d-ac96-774b-bcce-b302099a9%03d", number), kind: kind, status: status,
            asset: asset, usdcMicros: micros, units: micros.map { $0 * 4 }, actor: actor,
            txSignature: status == .pending ? nil : "sample-signature-\(number)",
            occurredAt: Date(timeIntervalSince1970: TimeInterval(1_790_996_400 - number * 3_600)))
    }
}

extension Components.Schemas.CabalActivityPage {
    public static let sampleFirst = Self(items: Components.Schemas.CabalActivity.samples, nextCursor: nil)

    public static let sampleEmpty = Self(items: [], nextCursor: nil)
}
#endif
