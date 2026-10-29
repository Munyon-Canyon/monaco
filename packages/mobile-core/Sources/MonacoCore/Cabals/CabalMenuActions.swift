import Foundation

public struct CabalMenuAction: Identifiable, Equatable, Sendable {
    public enum Kind: Equatable, Sendable {
        case vote(proposalID: String)
        case openCabal
        case fund
        case propose
        case invite
        case share
    }

    public enum Group: Equatable, Sendable {
        case trade
        case people
    }

    public let kind: Kind
    public let title: String
    public let group: Group

    public var id: String {
        switch kind {
        case .vote(let proposalID): "vote-\(proposalID)"
        case .openCabal: "vote-many"
        case .fund: "fund"
        case .propose: "propose"
        case .invite: "invite"
        case .share: "share"
        }
    }
}

public enum CabalMenuActions {
    public static func actions(canVote: Bool, restricted: Bool, waiting: [HomePendingVote]) -> [CabalMenuAction] {
        var actions: [CabalMenuAction] = []
        if let vote = voteAction(waiting) { actions.append(vote) }
        if !restricted { actions.append(CabalMenuAction(kind: .fund, title: "Fund", group: .trade)) }
        if canVote { actions.append(CabalMenuAction(kind: .propose, title: "Propose buy", group: .trade)) }
        actions.append(CabalMenuAction(kind: .invite, title: "Invite", group: .people))
        actions.append(CabalMenuAction(kind: .share, title: "Share", group: .people))
        return actions
    }

    public static func figuresLine(valueMicros: Int64, returnText: String) -> String {
        [CabalHeaderCopy.compactDollars(valueMicros), returnText].joined(separator: " \u{00B7} ")
    }

    public static func membersLine(memberCount: Int32, role: String, canVote: Bool) -> String {
        var parts = [CabalCopy.memberCount(memberCount)]
        if let viewer = CabalHeaderCopy.viewer(role: role, canVote: canVote) { parts.append(viewer) }
        return parts.joined(separator: " \u{00B7} ")
    }

    private static func voteAction(_ waiting: [HomePendingVote]) -> CabalMenuAction? {
        guard let first = waiting.first else { return nil }
        guard waiting.count == 1 else {
            return CabalMenuAction(kind: .openCabal, title: "Vote on \(waiting.count) proposals", group: .trade)
        }
        return CabalMenuAction(kind: .vote(proposalID: first.id), title: "Vote on \(subject(first))", group: .trade)
    }

    private static func subject(_ vote: HomePendingVote) -> String {
        if vote.isRule { return "Rule change" }
        return "\(vote.isSell ? "Sell" : "Buy") \(AssetSymbolFormatter.display(vote.symbol ?? ""))"
    }
}
