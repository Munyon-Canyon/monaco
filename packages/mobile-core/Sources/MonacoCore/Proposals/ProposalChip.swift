import Foundation

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

public enum ProposalBallot: String, Equatable, Sendable {
    case yes
    case no

    public var label: String { rawValue.capitalized }
}

public enum ProposalState: Equatable, Sendable {
    case open
    case voted(ProposalBallot)
    case viewOnly
    case approved
    case executed
    case expired
    case blocked
    case rejected
    case withdrawn
    case voided

    public init(status: ProposalStatus, ballot: String?, canVote: Bool, swapFailed: Bool) {
        if swapFailed {
            self = .blocked
            return
        }
        switch status {
        case .open:
            if let ballot = ballot.flatMap(ProposalBallot.init(rawValue:)) {
                self = .voted(ballot)
            } else {
                self = canVote ? .open : .viewOnly
            }
        case .passed: self = .approved
        case .executed: self = .executed
        case .failed: self = .rejected
        case .expired: self = .expired
        case .withdrawn: self = .withdrawn
        case .voided: self = .voided
        case .executionBlocked: self = .blocked
        }
    }

    public var isOpen: Bool {
        switch self {
        case .open, .voted, .viewOnly: true
        default: false
        }
    }
}

extension ProposalSummary {
    public func state(now: Date) -> ProposalState {
        ProposalState(
            status: status, ballot: myBallot, canVote: canVote && expiresAt > now, swapFailed: swap?.status == "failed")
    }
}

public struct ProposalBadge: Equatable, Sendable {
    public enum Tone: Equatable, Sendable { case brand, warn, negative, neutral }

    public let label: String
    public let tone: Tone

    private init(_ label: String, _ tone: Tone) {
        self.label = label
        self.tone = tone
    }

    public static func hero(_ state: ProposalState) -> Self {
        switch state {
        case .open: Self("Voting open", .brand)
        case .voted(let ballot): Self("You voted \(ballot.label)", .brand)
        case .viewOnly: Self("View only", .neutral)
        case .approved: Self("Approved · Executing", .warn)
        case .executed: Self("Executed", .brand)
        case .expired: Self("Expired", .neutral)
        case .blocked: Self("Not executed", .negative)
        case .rejected: Self("Not approved", .neutral)
        case .withdrawn: Self("Withdrawn", .neutral)
        case .voided: Self("Voided by Monaco", .neutral)
        }
    }

    public static func card(_ state: ProposalState) -> Self {
        switch state {
        case .open: Self("Needs your vote", .brand)
        case .viewOnly: Self("Voting open", .brand)
        case .approved: Self("Execution pending", .warn)
        case .blocked: Self("Trade not completed", .negative)
        case .voted, .executed, .expired, .rejected, .withdrawn, .voided: hero(state)
        }
    }

    public static func listRow(_ state: ProposalState) -> Self {
        switch state {
        case .open, .voted, .viewOnly: Self("Open", .brand)
        case .approved: Self("Pending", .warn)
        case .executed: Self("Executed", .brand)
        case .expired: Self("Expired", .negative)
        case .blocked: Self("Blocked", .negative)
        case .rejected: Self("Not approved", .neutral)
        case .withdrawn: Self("Withdrawn", .neutral)
        case .voided: Self("Voided", .neutral)
        }
    }

    public static func ballot(_ ballot: ProposalBallot?) -> Self {
        switch ballot {
        case .yes: Self("Yes", .brand)
        case .no: Self("No", .negative)
        case nil: Self("Not voted", .neutral)
        }
    }
}

public struct ProposalStepper: Equatable, Sendable {
    public enum Mark: Equatable, Sendable { case done, active, pending, failed }

    public struct Stamp: Equatable, Sendable {
        public let prefix: String
        public let date: Date

        public var text: String { "\(prefix) \(date.formatted(date: .abbreviated, time: .shortened))" }
    }

    public struct Step: Equatable, Sendable {
        public let title: String
        public let mark: Mark
        public let stamp: Stamp?
        public let note: String?
    }

    public let steps: [Step]
    public let isTerminal: Bool

    public var accessibilityLabel: String {
        if isTerminal, let step = steps.first {
            return ([step.title, step.stamp?.text, step.note].compactMap { $0 }).joined(separator: ". ")
        }
        let total = steps.count
        return steps.enumerated().map { index, step in
            let state =
                switch step.mark {
                case .done: "done"
                case .active: "in progress"
                case .pending: "to do"
                case .failed: "failed"
                }
            let detail = [step.stamp?.text, step.note].compactMap { $0 }.joined(separator: ", ")
            return "\(step.title), step \(index + 1) of \(total), \(state)" + (detail.isEmpty ? "" : ", \(detail)")
        }.joined(separator: ". ")
    }

    public static func make(
        status: ProposalStatus, isSell: Bool, swapFailed: Bool = false, expiresAt: Date,
        failureMessage: String? = nil, statusMessage: String? = nil, priceMoved: Bool = false
    ) -> Self {
        let action = isSell ? "sell" : "buy"
        let doing = isSell ? "Selling" : "Buying"
        let done = isSell ? "Sold" : "Bought"
        func voting(_ mark: Mark) -> Step {
            let stamp = mark == .active ? Stamp(prefix: "Closes", date: expiresAt) : nil
            return Step(title: "Voting", mark: mark, stamp: stamp, note: nil)
        }
        func step(_ title: String, _ mark: Mark, note: String? = nil) -> Step {
            Step(title: title, mark: mark, stamp: nil, note: note)
        }
        func terminal(_ title: String, closed: Bool, note: String?) -> Self {
            let stamp = closed ? Stamp(prefix: "Voting closed", date: expiresAt) : nil
            return Self(steps: [Step(title: title, mark: .failed, stamp: stamp, note: note)], isTerminal: true)
        }
        func tradeFailed() -> Self {
            let note: String?
            if swapFailed {
                let kept = isSell ? "The shares are still in the pot." : "The money is still in the pot."
                if priceMoved {
                    note = "The price moved past the cabal's limit since the vote. " + kept
                } else if let message = failureMessage ?? statusMessage {
                    note = (message.hasSuffix(".") ? message : message + ".") + " " + kept
                } else {
                    note = kept
                }
            } else {
                note = statusMessage
            }
            return Self(steps: [voting(.done), step("Couldn't \(action)", .failed, note: note)], isTerminal: false)
        }
        if swapFailed { return tradeFailed() }
        switch status {
        case .open:
            return Self(steps: [voting(.active), step(doing, .pending), step(done, .pending)], isTerminal: false)
        case .passed:
            return Self(steps: [voting(.done), step(doing, .active), step(done, .pending)], isTerminal: false)
        case .executed:
            return Self(steps: [voting(.done), step(doing, .done), step(done, .done)], isTerminal: false)
        case .expired:
            return terminal("Expired", closed: true, note: "Without enough yes votes.")
        case .failed:
            return terminal("Didn't pass", closed: true, note: "Not enough yes votes to pass.")
        case .withdrawn:
            return terminal("Withdrawn", closed: false, note: "The proposer took it back before voting closed.")
        case .voided:
            return terminal("Voided by Monaco", closed: false, note: statusMessage)
        case .executionBlocked:
            return tradeFailed()
        }
    }
}
