import Foundation
import MonacoAPI

public struct CabalRulesSummary: Equatable, Sendable {
    public struct Row: Equatable, Sendable, Identifiable {
        public let id: String
        public let title: String
        public let value: String
    }

    public let join: Row
    public let voters: Row
    public let threshold: Row
    public let expiry: Row

    public var rows: [Row] { [join, voters, threshold, expiry] }

    public init(_ cabal: Components.Schemas.Cabal) {
        let rules = cabal.rules
        join = Row(id: "join", title: "Who can join", value: Self.join(rules.joinMode))
        voters = Row(id: "voters", title: "Who votes", value: Self.voters(cabal))
        threshold = Row(
            id: "threshold", title: "To pass",
            value: CabalThreshold(rawValue: rules.threshold)?.label ?? rules.threshold)
        expiry = Row(
            id: "expiry", title: "Votes stay open",
            value: CabalProposalExpiry(rawValue: rules.proposalExpirySeconds)?.label
                ?? String(rules.proposalExpirySeconds))
    }

    private static func join(_ mode: String) -> String {
        switch CabalJoinMode(rawValue: mode) {
        case .open: "Anyone"
        case .request: "The creator approves"
        case nil: mode
        }
    }

    private static func voters(_ cabal: Components.Schemas.Cabal) -> String {
        switch CabalVoterMode(rawValue: cabal.rules.voterMode) {
        case .everyone: return "Every member"
        case nil: return cabal.rules.voterMode
        case .justMe:
            let voting = cabal.members.filter(\.canVote)
            let creatorID = cabal.creator.userId
            let creatorFirst = voting.filter { $0.userId == creatorID } + voting.filter { $0.userId != creatorID }
            return joined(creatorFirst.map(\.shownName))
        }
    }

    private static func joined(_ names: [String]) -> String {
        guard let last = names.last else { return "" }
        guard names.count > 1 else { return last }
        return names.dropLast().joined(separator: ", ") + " and " + last
    }
}

extension Components.Schemas.CabalMember {
    public var shownName: String {
        if !displayName.isEmpty { return displayName }
        return handle.map { "@\($0)" } ?? ""
    }
}
