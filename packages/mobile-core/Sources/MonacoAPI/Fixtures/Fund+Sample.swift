#if DEBUG
import Foundation

extension Components.Schemas.FundAccepted {
    public static let sample = Self(transferId: "01890a5d-ac96-774b-bcce-b302099a8057", status: .submitted)
}

extension Components.Schemas.FundTransfer {
    public static let sample = Self(status: .settled, amountMicros: "5000000", shareUnits: "5000000", failCode: nil)
}
#endif
