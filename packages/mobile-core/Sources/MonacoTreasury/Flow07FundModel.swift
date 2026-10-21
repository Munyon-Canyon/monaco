import MonacoAPI
import MonacoFlows

public enum FundFailure: Equatable, Sendable {
    case needsMoney(String)
    case toast(String)

    public static let notEnoughBalance = "Not enough in your account balance."
    public static let paused = "Trading in this cabal is paused. You can fund it again once it resumes."
    public static let notMember = "Join this cabal to fund it."
    public static let invalidAmount = "The minimum is $1."

    public init(_ error: APIError) {
        let serverMessage = ToastCopy.message(for: error)
        switch Flow07Outcome(error) {
        case .insufficientFunds:
            self = .needsMoney(Self.notEnoughBalance)
        case .cabalPaused:
            self = .toast(Self.paused)
        case .notCabalMember:
            self = .toast(Self.notMember)
        case .invalidInput:
            self = .toast(Self.invalidAmount)
        case .potValueZero:
            self = .toast(serverMessage)
        case .privyUnavailable:
            self = .toast(serverMessage)
        case .ok, .interrupted, nil:
            self = .toast(serverMessage)
        }
    }
}
