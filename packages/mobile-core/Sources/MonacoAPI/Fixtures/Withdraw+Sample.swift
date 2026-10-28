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

extension Components.Schemas.WithdrawalPreview {
    public static let sampleEmpty = Self(
        availableMicros: "500000000", feeMicros: "500000", feeKnown: true, feeMode: .onTop,
        maxAmountMicros: "499500000", receiveMicros: "0", overMicros: nil, belowMin: false,
        minMicros: "1000000", arrivalSeconds: 60, expiresAt: nil)
    public static let sampleTyped = Self(
        availableMicros: "500000000", feeMicros: "500000", feeKnown: true, feeMode: .onTop,
        maxAmountMicros: "499500000", receiveMicros: "120000000", overMicros: "0", belowMin: false,
        minMicros: "1000000", arrivalSeconds: 60, expiresAt: nil)
    public static let sampleOver = Self(
        availableMicros: "500000000", feeMicros: "500000", feeKnown: true, feeMode: .onTop,
        maxAmountMicros: "499500000", receiveMicros: "0", overMicros: "150500000", belowMin: false,
        minMicros: "1000000", arrivalSeconds: 60, expiresAt: nil)
    public static let sampleEVM = Self(
        availableMicros: "500000000", feeMicros: "7500", feeKnown: true, feeMode: .included,
        maxAmountMicros: "500000000", receiveMicros: "9992500", overMicros: "0", belowMin: false,
        minMicros: "1000000", arrivalSeconds: 120, expiresAt: Date(timeIntervalSince1970: 1_759_579_230))
    public static let sampleEVMFeeUnknown = Self(
        availableMicros: "500000000", feeMicros: nil, feeKnown: false, feeMode: .included,
        maxAmountMicros: "500000000", receiveMicros: "0", overMicros: nil, belowMin: false,
        minMicros: "1000000", arrivalSeconds: 120, expiresAt: nil)
}
#endif
