import MonacoAPI

public enum DeleteAccountBlocker: Equatable, Sendable {
    case cashOutFirst
    case withdrawFirst

    public var message: String {
        switch self {
        case .cashOutFirst: AccountCopy.cashOutFirst
        case .withdrawFirst: AccountCopy.withdrawFirst
        }
    }
}

public enum DeleteAccountState {
    public static func state(for error: APIError?) -> DeleteAccountBlocker? {
        guard case .problem(let problem) = error else { return nil }
        switch problem.code {
        case .known(.accountHasPositions): return .cashOutFirst
        case .known(.accountHasBalance): return .withdrawFirst
        default: return nil
        }
    }
}
