#if DEBUG
import Foundation

extension Components.Schemas.FundAccepted {
    public static let sample = Self(transferId: "01890a5d-ac96-774b-bcce-b302099a8057", status: .submitted)
}

extension Components.Schemas.FundTransfer {
    public static let sample = Self(
        status: .settled, amountMicros: "5000000", shareUnits: "5000000", sliceBps: 3600, failCode: nil)
}

extension Components.Schemas.FundPreview {
    public static let sample = Self(
        availableMicros: "500000000", minMicros: "1000000", sliceBpsNow: 3500, sliceBpsAfter: nil,
        overMicros: "0", paused: false, firstFundedAt: Date(timeIntervalSince1970: 1_759_579_200))
    public static let sampleTyped = Self(
        availableMicros: "500000000", minMicros: "1000000", sliceBpsNow: 3500, sliceBpsAfter: 3600,
        overMicros: "0", paused: false, firstFundedAt: Date(timeIntervalSince1970: 1_759_579_200))
    public static let sampleOver = Self(
        availableMicros: "500000000", minMicros: "1000000", sliceBpsNow: 3500, sliceBpsAfter: 3658,
        overMicros: "150000000", paused: false, firstFundedAt: Date(timeIntervalSince1970: 1_759_579_200))
    public static let samplePaused = Self(
        availableMicros: "500000000", minMicros: "1000000", sliceBpsNow: 3500, sliceBpsAfter: nil,
        overMicros: "0", paused: true, firstFundedAt: Date(timeIntervalSince1970: 1_759_579_200))
}
#endif
