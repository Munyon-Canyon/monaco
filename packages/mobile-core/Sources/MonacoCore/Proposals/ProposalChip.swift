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

public enum ProposalStepper: Equatable, Sendable {
    case voting
    case trading
    case done
    case failed(String)

    public static func state(status: ProposalStatus, isSell: Bool, swapFailed: Bool = false) -> Self {
        let action = isSell ? "sell" : "buy"
        if swapFailed || status == .executionBlocked { return .failed("Couldn't \(action)") }
        return switch status {
        case .open: .voting
        case .passed: .trading
        case .executed: .done
        case .failed, .expired, .withdrawn, .voided:
            .failed(ProposalChip.label(status: status, isSell: isSell) ?? "Couldn't \(action)")
        case .executionBlocked: .failed("Couldn't \(action)")
        }
    }

    public func trackerLabel(isSell: Bool) -> String {
        let steps = ProposalFeedCopy.trackerSteps(isSell: isSell)
        let index: Int
        switch self {
        case .voting: index = 0
        case .trading: index = 1
        case .done: index = 2
        case .failed(let title): return title
        }
        return "\(steps[index]), step \(index + 1) of \(steps.count)"
    }
}
