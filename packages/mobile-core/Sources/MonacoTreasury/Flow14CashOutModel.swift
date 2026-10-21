import MonacoAPI
import MonacoFlows

public enum CashOutRefusal: Equatable, Sendable {
    case paused
    case inProgress
    case sliceChanged
    case invalidRequest
    case unconfirmed
    case message(String)

    public init(_ error: APIError) {
        let serverMessage = ToastCopy.message(for: error)
        switch Flow14Outcome(error) {
        case .cabalPaused:
            self = .paused
        case .cashOutInProgress:
            self = .inProgress
        case .potValueChanged:
            self = .sliceChanged
        case .insufficientShares:
            self = .sliceChanged
        case .priceUnavailable:
            self = .sliceChanged
        case .invalidInput:
            self = .invalidRequest
        case .privyUnavailable:
            self = .message(serverMessage)
        case .rPCUnavailable:
            self = .message(serverMessage)
        case .saleShort:
            self = .message(serverMessage)
        case .ok, .interrupted, nil:
            switch error {
            case .transport, .decoding, .cancelled: self = .unconfirmed
            case .problem, .signedOut, .missingAccessToken, .accountDeleted, .inFlight: self = .message(serverMessage)
            }
        }
    }
}
