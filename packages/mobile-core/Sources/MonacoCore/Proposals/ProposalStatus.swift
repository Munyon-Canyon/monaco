import MonacoAPI

public enum ProposalStatus: String, CaseIterable, Sendable {
    case open
    case passed
    case executed
    case failed
    case expired
    case withdrawn
    case voided
    case executionBlocked = "execution_blocked"

    public init(_ status: Components.Schemas.ProposalStatus) {
        switch status {
        case .open: self = .open
        case .passed: self = .passed
        case .executed: self = .executed
        case .failed: self = .failed
        case .expired: self = .expired
        case .withdrawn: self = .withdrawn
        case .voided: self = .voided
        case .executionBlocked: self = .executionBlocked
        }
    }
}

public enum ProposalKind: String, Sendable {
    case buy
    case sell

    public init(_ kind: Components.Schemas.ProposalKind) {
        switch kind {
        case .buy: self = .buy
        case .sell: self = .sell
        }
    }

    public var title: String {
        switch self {
        case .buy: "Buy"
        case .sell: "Sell"
        }
    }

    var verb: String {
        switch self {
        case .buy: "buy"
        case .sell: "sell"
        }
    }
}

public enum BallotChoice: String, CaseIterable, Sendable {
    case yes
    case no
}

public enum SwapState: String, Sendable {
    case created
    case submitted
    case confirmed
    case failed
}
