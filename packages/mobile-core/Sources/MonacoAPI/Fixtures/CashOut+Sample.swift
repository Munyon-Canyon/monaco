#if DEBUG
import Foundation

extension Components.Schemas.CashOutPreview {
    public static let sample = Self(sliceMicros: "200150000", shareUnits: "200150000", minMicros: "100000", pause: nil)
    public static let sampleNoStake = Self(sliceMicros: "0", shareUnits: "0", minMicros: "100000", pause: nil)
    public static let sampleOpsPause = Self(
        sliceMicros: "200150000", shareUnits: "200150000", minMicros: "100000",
        pause: .init(reasons: ["ops"], since: Date(timeIntervalSince1970: 1_759_579_200))
    )
    public static let sampleDepositPause = Self(
        sliceMicros: "200150000", shareUnits: "200150000", minMicros: "100000",
        pause: .init(reasons: ["external_deposit"], since: Date(timeIntervalSince1970: 1_759_579_200))
    )
}

extension Components.Schemas.CashOutJob {
    public static func sample(
        status: StatusPayload, payoutMicros: String = "1000000", resultCode: String? = nil
    ) -> Self {
        Self(
            id: "01890a5d-ac96-774b-bcce-b302099a8070",
            cabalId: "01890a5d-ac96-774b-bcce-b302099a8059",
            userId: "01890a5d-ac96-774b-bcce-b302099a8058",
            shareUnits: "1000000",
            payoutMicros: payoutMicros,
            sellUsdcMicros: "0",
            status: status,
            resultCode: resultCode,
            createdAt: Date(timeIntervalSince1970: 1_759_579_200),
            updatedAt: Date(timeIntervalSince1970: 1_759_579_200)
        )
    }
}
#endif
