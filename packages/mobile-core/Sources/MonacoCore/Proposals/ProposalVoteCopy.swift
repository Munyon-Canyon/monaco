import Foundation

public struct CastOutcome: Equatable, Sendable {
    public let choice: ProposalBallot
    public let status: ProposalStatus
    public let tally: ProposalTally
    public let undoUntil: Date?
    public let myVote: ProposalMyVote?
    public let changed: Bool

    public var settled: Bool { status == .passed || status == .failed }

    public init(
        choice: ProposalBallot, status: ProposalStatus, tally: ProposalTally, undoUntil: Date?,
        myVote: ProposalMyVote?, changed: Bool = false
    ) {
        self.choice = choice
        self.status = status
        self.tally = tally
        self.undoUntil = undoUntil
        self.myVote = myVote
        self.changed = changed
    }
}

public struct VoteToast: Equatable, Sendable {
    public let message: String
    public let offersUndo: Bool
}

public struct VoteCommittedLine: Equatable, Sendable {
    public let title: String
    public let yes: Int
    public let no: Int
    public let accessibilityLabel: String
}

public enum ProposalVoteHint: Equatable, Sendable {
    case picked, holding
}

public enum ProposalVoteCopy {
    public static let undone = "Vote undone."

    public static func statusLine(tally: ProposalTally, replies: Int, expiresAt: Date, now: Date) -> String {
        statusFacts(tally: tally, replies: replies, expiresAt: expiresAt, now: now).joined(separator: " · ")
    }

    public static func statusFacts(tally: ProposalTally, replies: Int, expiresAt: Date, now: Date) -> [String] {
        var facts = ["\(tally.yes + tally.no) of \(tally.voters) voted"]
        if replies > 0 { facts.append(replies == 1 ? "1 reply" : "\(replies) replies") }
        facts.append(endsIn(expiresAt, now: now))
        return facts
    }

    public static func endsIn(_ expiresAt: Date, now: Date) -> String {
        let remaining = Int(expiresAt.timeIntervalSince(now))
        if remaining <= 0 { return "voting closed" }
        let days = remaining / 86_400
        let hours = remaining / 3600
        if days >= 2 { return "ends in \(days)d" }
        if hours > 0 { return "ends in \(hours)h" }
        return "ends in \(max(remaining / 60, 1))m"
    }

    public static func hint(_ hint: ProposalVoteHint) -> String {
        switch hint {
        case .picked: "Hold to vote · lift to cancel"
        case .holding: "Keep holding · lift to cancel"
        }
    }

    public static func committedOrdinary(choice: ProposalBallot, tally: ProposalTally) -> VoteCommittedLine {
        let title = "You voted \(choice.label)"
        return VoteCommittedLine(
            title: title, yes: tally.yes, no: tally.no,
            accessibilityLabel: "\(title). \(tally.yes) yes, \(tally.no) no.")
    }

    public static func settledLine(
        status: ProposalStatus, tally: ProposalTally, voted: Int, voters: Int, sharesEstimate: String?,
        isSell: Bool = false, unit: String = "sh"
    ) -> String {
        settledFacts(
            status: status, tally: tally, voted: voted, voters: voters, sharesEstimate: sharesEstimate, isSell: isSell,
            unit: unit
        ).joined(separator: " · ")
    }

    public static func settledFacts(
        status: ProposalStatus, tally: ProposalTally, voted: Int, voters: Int, sharesEstimate: String?,
        isSell: Bool = false, unit: String = "sh"
    ) -> [String] {
        let score = "\(tally.yes)–\(tally.no)"
        let turnout = "\(voted) of \(voters) voted"
        guard status == .passed else { return ["Failed \(score)", turnout, "no trade"] }
        var facts = ["Passed \(score)", turnout]
        if let sharesEstimate { facts.append("\(isSell ? "selling" : "buying") \(sharesEstimate) \(unit)") }
        return facts
    }

    public static func decidingLine(myVote: ProposalMyVote, needed: Int, voters: Int) -> String? {
        guard myVote.decided else { return nil }
        if myVote.yesOrder > 0 {
            return "Yours was the \(ordinal(myVote.yesOrder)) yes · at \(needed) of \(voters) it passes"
        }
        return "Yours was the \(ordinal(myVote.noOrder)) no · it can't pass now"
    }

    public static func readOnlyLabel(cabalName: String) -> String { "\(cabalName) members vote" }

    public static func readOnlyVoterLabel(cabalName _: String) -> String { "Voters from when it opened vote" }

    public static func readOnlyAccessibilityLabel(tally: ProposalTally, middle: String) -> String {
        "\(tally.yes) yes, \(tally.no) no. \(middle). \(tally.yes + tally.no) of \(tally.voters) voted."
    }

    public static func toast(outcome: CastOutcome) -> VoteToast {
        let voted = "Voted \(outcome.choice.label)."
        if outcome.settled {
            let score = "\(outcome.tally.yes)–\(outcome.tally.no)"
            let result = outcome.status == .passed ? "Passed" : "Failed"
            return VoteToast(message: "\(voted) \(result) \(score).", offersUndo: false)
        }
        if outcome.changed {
            return VoteToast(message: "Changed to \(outcome.choice.label).", offersUndo: false)
        }
        guard let myVote = outcome.myVote else {
            return VoteToast(message: voted, offersUndo: outcome.undoUntil != nil)
        }
        return VoteToast(
            message: "\(voted) \(ordinal(myVote.order)) of \(myVote.of).", offersUndo: outcome.undoUntil != nil)
    }

    public static func ordinal(_ value: Int) -> String {
        let lastTwo = value % 100
        let suffix =
            switch (lastTwo >= 11 && lastTwo <= 13, value % 10) {
            case (false, 1): "st"
            case (false, 2): "nd"
            case (false, 3): "rd"
            default: "th"
            }
        return "\(value)\(suffix)"
    }
}
