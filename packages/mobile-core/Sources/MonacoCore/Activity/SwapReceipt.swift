import Foundation
import MonacoAPI

public struct SwapReceipt: Equatable, Sendable {
    public let statusLabel: String
    public let assetLine: String
    public let amount: String?
    public let failureMessage: String?
    public let solscanURL: URL?
    public let retryable: Bool

    public init(_ swap: Components.Schemas.SwapDetail) {
        self.statusLabel =
            switch swap.status {
            case .created, .submitted: "Pending"
            case .confirmed: "Done"
            case .failed: "Failed"
            }
        self.assetLine = "\(swap.assetName) · \(AssetSymbolFormatter.display(swap.symbol))"
        self.amount = swap.usdcMicros.map { UsdAmountFormatter.format(micros: $0) }
        self.failureMessage = swap.status == .failed ? swap.failureMessage : nil
        self.solscanURL = swap.txSignature.flatMap { URL(string: "https://solscan.io/tx/\($0)") }
        self.retryable = swap.retryable
    }
}
