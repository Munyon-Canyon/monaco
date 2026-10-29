import Foundation
import MonacoAPI

public struct InviteLandingContent: Equatable, Sendable {
    public struct Stats: Equatable, Sendable {
        public let treasury: String
        public let oneMonth: String
        public let oneMonthIsLoss: Bool
        public let turnout: String
    }

    public struct Proposal: Equatable, Sendable {
        public let title: String
        public let byline: String
        public let logoURL: String?
        public let symbol: String
        public let assetName: String
        public let yesText: String
        public let noText: String
        public let voteNote: String
        public let detail: String
        public let yes: Int
        public let no: Int
    }

    public let name: String
    public let pictureURL: String?
    public let facts: String
    public let inviter: CabalInvitePreview.Person?
    public let inviterLine: String?
    public let faces: [CabalInvitePreview.Person]
    public let membersLine: String?
    public let stats: Stats
    public let proposal: Proposal?
    public let joinMode: CabalJoinPolicy
}

public enum InviteLandingCopy {
    public static let signedOutNote = "You'll sign in with your phone to join."
    public static let requestSentTitle = "Request sent"
    public static let voteNote = "Join to vote"
    public static let emptyFigure = "\u{2014}"
    public static let bannedToast = "This cabal isn't taking members."
    public static let pendingToast = "Your request is waiting."

    private static let months = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"]

    public static func content(_ source: Components.Schemas.ReferralCabal, now: Date) -> InviteLandingContent? {
        guard let cabal = source.cabal, let base = CabalInviteCopy.preview(source, now: now) else { return nil }
        return InviteLandingContent(
            name: cabal.name, pictureURL: cabal.pictureUrl,
            facts: facts(memberCount: Int(cabal.memberCount), since: cabal.since),
            inviter: base.inviter, inviterLine: base.inviterLine, faces: base.faces,
            membersLine: membersLine(names: base.faces.map(\.displayName), moreCount: base.moreCount),
            stats: InviteLandingContent.Stats(
                treasury: CabalInviteCopy.wholeDollars(micros: cabal.potValueMicros),
                oneMonth: source.return1mBps.map {
                    PercentFormatter.format(basisPoints: Int64($0), signed: true, fractionDigits: 1)
                }
                    ?? emptyFigure,
                oneMonthIsLoss: (source.return1mBps ?? 0) < 0,
                turnout: source.turnoutMonthBps.map {
                    PercentFormatter.format(basisPoints: Int64($0), signed: false, fractionDigits: 0)
                }
                    ?? emptyFigure),
            proposal: source.openProposal.flatMap { proposal in
                base.proposal.map { shown in
                    InviteLandingContent.Proposal(
                        title: shown.title,
                        byline: "\(CabalInviteCopy.firstName(proposal.proposerName)) \u{00B7} \(proposal.title)",
                        logoURL: shown.logoURL, symbol: shown.symbol, assetName: shown.assetName,
                        yesText: "\u{2713} \(shown.yes)", noText: "\(shown.no) \u{2715}", voteNote: voteNote,
                        detail: shown.detail, yes: shown.yes, no: shown.no)
                }
            },
            joinMode: CabalJoinPolicy(wire: cabal.joinMode.rawValue))
    }

    public static func facts(memberCount: Int, since: String) -> String {
        let members = "\(memberCount) \(memberCount == 1 ? "member" : "members")"
        return "Cabal \u{00B7} \(members) \u{00B7} since \(monthYear(since))"
    }

    public static func monthYear(_ day: String) -> String {
        let parts = day.split(separator: "-")
        guard parts.count >= 2, let month = Int(parts[1]), (1...12).contains(month) else { return day }
        return "\(months[month - 1]) \(parts[0])"
    }

    public static func membersLine(names: [String], moreCount: Int) -> String? {
        let firsts = names.map(CabalInviteCopy.firstName)
        guard let last = firsts.last else { return nil }
        if moreCount > 0 { return "\(firsts.joined(separator: ", ")) and \(moreCount) more" }
        if firsts.count == 1 { return last }
        return "\(firsts.dropLast().joined(separator: ", ")) and \(last)"
    }

    public static func buttonTitle(name: String, policy: CabalJoinPolicy) -> String {
        policy == .request ? "Ask to join \(name)" : "Join \(name)"
    }

    public static func requestSentToast(name: String) -> String { "Request sent to \(name)." }
}
