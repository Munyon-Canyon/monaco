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
        self = Self(rawValue: status.rawValue) ?? .open
    }
}
