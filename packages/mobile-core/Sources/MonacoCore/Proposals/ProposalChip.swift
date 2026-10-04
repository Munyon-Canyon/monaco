public enum ProposalChip {
    public static func label(status: ProposalStatus, kind: ProposalKind, swap: SwapState? = nil) -> String? {
        if status == .executionBlocked || swap == .failed { return "Couldn't \(kind.verb)" }
        return switch status {
        case .open: nil
        case .passed: kind == .sell ? "Selling" : "Buying"
        case .executed: kind == .sell ? "Sold" : "Bought"
        case .failed: "Didn't pass"
        case .expired: "Expired"
        case .withdrawn: "Withdrawn"
        case .voided: "Voided by Monaco"
        case .executionBlocked: nil
        }
    }
}

public struct ProposalSteps: Equatable, Sendable {
    public let titles: [String]
    public let reached: Int
    public let failed: Bool
    public let failureMessage: String?

    public init(status: ProposalStatus, kind: ProposalKind, swap: SwapState?, failureMessage: String?) {
        let trading = kind == .sell ? "Selling" : "Buying"
        if status == .executionBlocked || swap == .failed {
            titles = ["Voting", trading, "Couldn't \(kind.verb)"]
            reached = 2
            failed = true
            self.failureMessage = failureMessage
            return
        }
        titles = ["Voting", trading, "Done"]
        failed = false
        self.failureMessage = nil
        reached =
            switch status {
            case .executed: 2
            case .passed: swap == .confirmed ? 2 : 1
            case .open, .failed, .expired, .withdrawn, .voided, .executionBlocked: 0
            }
    }
}
