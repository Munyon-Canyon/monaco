import Foundation
import MonacoAPI

public enum CabalMemberGroup: Equatable, Sendable {
    case admin
    case voter
    case member

    public init(_ row: Components.Schemas.CabalMemberRow) {
        switch (row.role, row.canVote) {
        case (.creator, _), (.admin, _): self = .admin
        case (_, true): self = .voter
        default: self = .member
        }
    }

    public var word: String {
        switch self {
        case .admin: "Admin"
        case .voter: "Voter"
        case .member: "Member"
        }
    }

    public var plural: String {
        switch self {
        case .admin: "Admins"
        case .voter: "Voters"
        case .member: "Members"
        }
    }

    public var votes: Bool { self != .member }
}

public enum CabalMemberCopy {
    public static let membersFooter =
        "Voters decide; members hold a share. A right call is a vote that matched how the trade did, counted once it resolves. Turnout is the votes cast of the votes open. Make a member a voter by proposal."
    public static let votersFooter = "Voters decide on every trade and rule. Change who votes by proposal."
    public static let invite = "Invite to this cabal"
    public static let approve = "Approve"
    public static let decline = "Decline"
    public static let noVotes = "No votes this month"
    public static let holdsAShare = "Holds a share"
    public static let youSuffix = "you"
    public static let declinedToast = "Declined."

    public static func recordLine(_ month: Components.Schemas.MemberMonth) -> String {
        if month.eligible == 0 { return noVotes }
        let voted = "voted \(month.voted) of \(month.eligible)"
        if month.resolved == 0 { return voted }
        return "\(month.right) of \(month.resolved) right \u{00B7} \(voted)"
    }

    public static func tabLine(_ row: Components.Schemas.CabalMemberRow) -> String {
        "\(CabalMemberGroup(row).word) \u{00B7} \(recordLine(row.month))"
    }

    public static func voterLine(_ row: Components.Schemas.CabalMemberRow) -> String {
        CabalMemberGroup(row).votes ? recordLine(row.month) : holdsAShare
    }

    public static func name(_ row: Components.Schemas.CabalMemberRow, viewerID: String?) -> String {
        let shown = row.displayName.isEmpty ? (row.handle.map { "@\($0)" } ?? "Someone") : row.displayName
        return row.userId == viewerID ? "\(shown) \u{00B7} \(youSuffix)" : shown
    }

    public static func share(basisPoints: Int32) -> String {
        if basisPoints > 0 && basisPoints < 100 { return "<1%" }
        return "\(basisPoints / 100)%"
    }

    public static func requestLine(via: String?, createdAt: Date, now: Date) -> String {
        let age = ProposalCardCopy.age(since: createdAt, now: now)
        guard let via else { return "Asked to join \u{00B7} \(age)" }
        return "Asked to join \u{00B7} \(via)'s link \u{00B7} \(age)"
    }

    public static func sectionTitle(_ group: CabalMemberGroup, totals: Components.Schemas.CabalMemberTotals) -> String {
        "\(group.plural) \(count(group, totals: totals))"
    }

    public static func count(_ group: CabalMemberGroup, totals: Components.Schemas.CabalMemberTotals) -> Int {
        switch group {
        case .admin: Int(totals.admins)
        case .voter: Int(totals.voters)
        case .member: Int(totals.members)
        }
    }

    public static func voteTitle(totals: Components.Schemas.CabalMemberTotals) -> String {
        "Vote (\(totals.admins + totals.voters))"
    }

    public static func dontVoteTitle(totals: Components.Schemas.CabalMemberTotals) -> String {
        "Don't vote (\(totals.members))"
    }

    public static func seeAll(_ count: Int32) -> String { "See all \(count)" }

    public static func inviteTo(_ name: String) -> String { "Invite to \(name)" }

    public static func joinedToast(name: String, cabal: String) -> String { "\(name) joined \(cabal)." }

    public static func voterAction(_ group: CabalMemberGroup) -> String {
        group.votes ? "Propose removing as voter" : "Propose making a voter"
    }

    public static let passCreatorAction = "Pass the creator role"

    public static func adminAction(_ group: CabalMemberGroup) -> String {
        group == .admin ? "Propose removing as admin" : "Propose making an admin"
    }
}
