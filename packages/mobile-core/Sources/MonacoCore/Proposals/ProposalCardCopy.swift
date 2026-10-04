import Foundation

public enum ProposalCardCopy {
    public static func closes(at expiry: Date, now: Date) -> String {
        let seconds = max(0, Int(expiry.timeIntervalSince(now)))
        if seconds >= 3600 { return "Closes in \(seconds / 3600)h" }
        return "Closes in \(max(1, seconds / 60))m"
    }

    public static func tracker(voted: Int, voters: Int, needed: Int) -> String {
        "\(voted) of \(voters) voted · \(needed) yes to pass"
    }

    public static func voter(_ name: String, choice: String?) -> String {
        guard let choice else { return "\(name) hasn't voted" }
        return "\(name) voted \(choice)"
    }

    public static func viewerVote(_ choice: String?) -> String? {
        choice.map { "✓ You voted \($0)" }
    }

    public static func sellAmount(atomics: String, decimals: Int, kind: AssetKind) -> String {
        let unit = kind == .preIpo ? "tokens" : "shares"
        return "\(ProposalShareFormatter.shares(fromAtomics: atomics, decimals: decimals)) \(unit)"
    }
}
