import Foundation
import MonacoAPI

public struct CabalRulesRows: Equatable, Sendable {
    public enum Kind: String, CaseIterable, Sendable {
        case votesToPass, voteWindow, maxPerTrade, whoCanPropose, cashOut, showSides
    }

    public struct Row: Equatable, Sendable, Identifiable {
        public let kind: Kind
        public let title: String
        public let value: String
        public var id: Kind { kind }
    }

    public static let footer =
        "Everyone sees the count as it happens. With this on, everyone also sees who voted which way once a vote closes. Rules change by proposal."

    public let rows: [Row]
    public let showSides: Bool
    public var footer: String { Self.footer }

    public init(_ cabal: Components.Schemas.Cabal) {
        let rules = cabal.rules
        showSides = rules.showSides
        rows = [
            Row(kind: .votesToPass, title: "Votes to pass", value: Self.votesToPass(cabal)),
            Row(kind: .voteWindow, title: "Vote window", value: Self.window(rules.proposalExpirySeconds)),
            Row(kind: .maxPerTrade, title: "Max per trade", value: Self.maxPerTrade(rules.maxTradeBps)),
            Row(kind: .whoCanPropose, title: "Who can propose", value: "Voters"),
            Row(kind: .cashOut, title: "Cash out", value: "Any time"),
            Row(kind: .showSides, title: "Show who voted what", value: ""),
        ]
    }

    static func votesToPass(_ cabal: Components.Schemas.Cabal) -> String {
        let voters = Int(cabal.voterCount)
        let needed = CabalThreshold(rawValue: cabal.rules.threshold) == .unanimous ? voters : voters / 2 + 1
        return "\(needed) of \(voters)"
    }

    static func window(_ seconds: Int32) -> String {
        switch CabalProposalExpiry(rawValue: seconds) {
        case .oneHour: "1 hour"
        case .oneDay: "24 hours"
        case .oneWeek: "7 days"
        case nil: wholeHours(seconds) ?? CabalRulesSummary.durationLabel(seconds: seconds)
        }
    }

    private static func wholeHours(_ seconds: Int32) -> String? {
        guard seconds >= 3_600, seconds < 172_800, seconds % 3_600 == 0 else { return nil }
        return seconds == 3_600 ? "1 hour" : "\(seconds / 3_600) hours"
    }

    static func maxPerTrade(_ bps: Int32?) -> String {
        guard let bps else { return "No limit" }
        let whole = bps / 100
        let hundredths = bps % 100
        guard hundredths != 0 else { return "\(whole)% of the cash" }
        let padded = hundredths < 10 ? "0\(hundredths)" : "\(hundredths)"
        return "\(whole).\(padded)% of the cash"
    }
}
