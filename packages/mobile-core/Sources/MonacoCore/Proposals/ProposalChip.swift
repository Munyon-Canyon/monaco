public enum ProposalChip {
    public static func label(status: ProposalStatus, isSell: Bool, swapFailed: Bool = false) -> String? {
        let action = isSell ? "sell" : "buy"
        if swapFailed || status == .executionBlocked { return "Couldn't \(action)" }
        return switch status {
        case .open: nil
        case .passed: isSell ? "Selling" : "Buying"
        case .executed: isSell ? "Sold" : "Bought"
        case .failed: "Didn't pass"
        case .expired: "Expired"
        case .withdrawn: "Withdrawn"
        case .voided: "Voided by Monaco"
        case .executionBlocked: nil
        }
    }
}
