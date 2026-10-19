import Foundation

public struct GroupCashSummary: Equatable, Sendable {
    public enum YieldStatus: Equatable, Sendable {
        case awaitingConfirmation
        case unavailable
        case rate(String)
    }

    public let cash: String
    public let asset = "USDC"
    public let yieldStatus: YieldStatus

    public init(_ pot: CabalPotSummary, yieldStatus: YieldStatus = .awaitingConfirmation) {
        cash = pot.cash
        self.yieldStatus = yieldStatus
    }

    public var yieldLabel: String {
        switch yieldStatus {
        case .awaitingConfirmation: "Rate requires live confirmation"
        case .unavailable: "Rate unavailable"
        case .rate(let rate): rate
        }
    }

    public var notice: ReceiptContent.Notice? {
        let message =
            "Current yield details are not available. The provider, rate and applicable terms will appear when confirmed."
        return switch yieldStatus {
        case .awaitingConfirmation: ReceiptContent.Notice(tone: .brand, message: message)
        case .unavailable: ReceiptContent.Notice(tone: .warn, message: message)
        case .rate: nil
        }
    }
}
