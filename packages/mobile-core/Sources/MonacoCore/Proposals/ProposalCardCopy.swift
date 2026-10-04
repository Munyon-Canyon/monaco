import Foundation

public enum ProposalCardCopy {
    public static func closes(at expiry: Date, now: Date) -> String {
        let seconds = max(0, Int(expiry.timeIntervalSince(now)))
        let hours = seconds / 3600
        if hours >= 48 { return "Closes in \(hours / 24)d" }
        if hours >= 1 { return "Closes in \(hours)h" }
        return "Closes in \(max(1, seconds / 60))m"
    }

    public static func age(since date: Date, now: Date) -> String {
        let seconds = max(0, Int(now.timeIntervalSince(date)))
        if seconds < 60 { return "now" }
        if seconds < 3600 { return "\(seconds / 60)m" }
        if seconds < 86_400 { return "\(seconds / 3600)h" }
        return "\(seconds / 86_400)d"
    }

    public static func tracker(voted: Int, voters: Int, needed: Int) -> String {
        "\(voted) of \(voters) voted · \(needed) yes to pass"
    }

    public static func voter(_ name: String, choice: BallotChoice?) -> String {
        guard let choice else { return "\(name) hasn't voted" }
        return "\(name) voted \(choice.rawValue)"
    }

    public static func viewerVote(_ choice: BallotChoice) -> String {
        "✓ You voted \(choice.rawValue)"
    }

    public static func proposedBy(_ name: String, since date: Date, now: Date) -> String {
        "Proposed by \(name) · \(age(since: date, now: now))"
    }

    public static func reasonTitle(_ kind: ProposalKind) -> String {
        "Why \(kind.verb)"
    }

    public static let voteToast = "Vote in"
}
