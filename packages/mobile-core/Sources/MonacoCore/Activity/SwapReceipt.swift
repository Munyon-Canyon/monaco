import Foundation
import MonacoAPI

public struct SwapReceipt: Equatable, Sendable {
    public let statusLabel: String
    public let assetLine: String
    public let amount: String?
    public let failureMessage: String?
    public let solscanURL: URL?
    public let retryable: Bool
    public let bestPrice: BestPrice?

    public init(_ swap: Components.Schemas.SwapDetail) {
        let hasOpenStep = swap.steps?.contains { $0.status == .pending || $0.status == .submitted } == true
        self.statusLabel =
            switch swap.status {
            case .failed: "Failed"
            case _ where hasOpenStep: "In progress"
            case .created, .submitted: "Pending"
            case .confirmed: "Done"
            }
        self.assetLine = "\(swap.assetName) · \(AssetSymbolFormatter.display(swap.symbol))"
        self.amount = swap.usdcMicros.map { UsdAmountFormatter.format(micros: $0) }
        self.failureMessage = swap.status == .failed ? swap.failureMessage : nil
        self.solscanURL = swap.txSignature.flatMap { URL(string: "https://solscan.io/tx/\($0)") }
        self.retryable = swap.retryable
        self.bestPrice = BestPrice(swap.route, isSell: swap.action == .sell)
    }
}
