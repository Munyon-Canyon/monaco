import Foundation
import OpenAPIRuntime

public struct ProposalRuleChange: Equatable, Sendable {
    public let rule: String
    public let from: String
    public let to: String
    public let target: String?

    public init(rule: String, from: String, to: String, target: String? = nil) {
        self.rule = rule
        self.from = from
        self.to = to
        self.target = target
    }

    init(
        rule: String, from: OpenAPIValueContainer, to: OpenAPIValueContainer, targetName: String?, targetHandle: String?
    ) {
        let name = targetName.flatMap { $0.isEmpty ? nil : $0 } ?? targetHandle.map { "@\($0)" }
        self.init(
            rule: rule, from: Self.valueLabel(rule: rule, value: from.value),
            to: Self.valueLabel(rule: rule, value: to.value), target: name)
    }

    public var title: String { ProposalRuleCopy.title(rule) }

    public var changeLine: String { "\(from) \u{2192} \(to)" }

    public var detailLine: String {
        guard let target else { return "\(title): \(changeLine)" }
        return "\(title) · \(target): \(changeLine)"
    }

    static func valueLabel(rule: String, value: (any Sendable)?) -> String {
        switch rule {
        case "show_sides":
            return (value as? Bool) == true ? "Shown" : "Hidden"
        case "max_trade_bps":
            guard let bps = value as? Int else { return "No limit" }
            return "\(bps / 100)% of cash"
        case "threshold":
            return (value as? String).flatMap { CabalThreshold(rawValue: $0)?.label } ?? "Unknown"
        case "proposal_expiry_seconds":
            guard let seconds = value as? Int else { return "Unknown" }
            return Int32(exactly: seconds).flatMap(CabalProposalExpiry.init(rawValue:))?.label ?? "\(seconds)s"
        case "join_mode":
            return CabalJoinPolicy(wire: (value as? String) ?? "").label
        case "voter":
            return (value as? Bool) == true ? "Can vote" : "Can't vote"
        case "admin":
            return (value as? Bool) == true ? "Admin" : "Not an admin"
        case "creator":
            return (value as? Bool) == true ? "Creator" : "Not the creator"
        default:
            return "Changed"
        }
    }
}

public enum ProposalRuleCopy {
    public static let heroTitle = "Change rule"
    public static let applied = "Applied"
    public static let appliedNote = "The rule change is in effect."
    public static let voteTitle = "Change rule"

    public static func title(_ rule: String?) -> String {
        switch rule {
        case "show_sides": "Show who voted what"
        case "max_trade_bps": "Max per trade"
        case "threshold": "Votes to pass"
        case "proposal_expiry_seconds": "Vote window"
        case "join_mode": "Who can join"
        case "voter": "Who can vote"
        case "admin": "Who can manage"
        case "creator": "Who leads the cabal"
        default: "Cabal rule"
        }
    }

    public static func caption(status: ProposalStatus, tally: ProposalTally) -> String? {
        switch status {
        case .passed, .executed: "rule applied"
        case .failed: "needed \(tally.needed) · rule unchanged"
        case .expired: "closed before \(tally.needed) yes · rule unchanged"
        case .executionBlocked, .withdrawn, .voided, .open: nil
        }
    }
}
