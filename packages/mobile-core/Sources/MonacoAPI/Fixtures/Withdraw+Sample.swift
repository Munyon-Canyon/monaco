#if DEBUG
import Foundation

extension Components.Schemas.WithdrawAccepted {
    public static let sample = Self(
        withdrawalId: "01890a5d-ac96-774b-bcce-b302099a8057",
        status: .submitted,
        txSignature: "signature-1"
    )
}

extension Components.Schemas.Withdrawal {
    public static let sample = Self(
        withdrawalId: "01890a5d-ac96-774b-bcce-b302099a8057",
        status: .confirmed,
        amountMicros: "2000000",
        toAddress: "wallet-2",
        txSignature: "signature-1",
        failCode: nil,
        createdAt: Date(timeIntervalSince1970: 1_759_579_200),
        completedAt: Date(timeIntervalSince1970: 1_759_579_210)
    )
}
#endif
