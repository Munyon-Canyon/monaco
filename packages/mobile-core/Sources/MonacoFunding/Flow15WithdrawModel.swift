import MonacoAPI
import MonacoFlows

public enum WithdrawFailure: Equatable, Sendable {
    case address(String)
    case toast(String)

    public static let invalidAddress = "Paste a wallet address. That one can't receive USDC from here."
    public static let ownWallet = "That's your own deposit address. Paste the address you want to send to."
    public static let insufficientFunds = "Not enough in your account balance."
    public static let invalidAmount = "The minimum is $1."

    public init(_ error: APIError) {
        let serverMessage = ToastCopy.message(for: error)
        switch Flow15Outcome(error) {
        case .invalidAddress:
            self = .address(Self.invalidAddress)
        case .withdrawToOwnWallet:
            self = .address(Self.ownWallet)
        case .insufficientFunds:
            self = .toast(Self.insufficientFunds)
        case .invalidInput:
            self = .toast(Self.invalidAmount)
        case .privyUnavailable, .stepUpRequired, .stepUpMismatch:
            self = .toast(serverMessage)
        case .ok, .interrupted, nil:
            self = .toast(serverMessage)
        }
    }
}
