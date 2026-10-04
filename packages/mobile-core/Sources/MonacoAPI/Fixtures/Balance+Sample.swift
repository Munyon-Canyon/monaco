#if DEBUG
import Foundation

extension Components.Schemas.Balance {
    public static let sample = Self(
        availableMicros: "248500000", onChainMicros: "298500000", inFlightMicros: "50000000",
        depositAddress: "wallet-1", asOf: Date(timeIntervalSince1970: 1_759_579_200)
    )
}
#endif
