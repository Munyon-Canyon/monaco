import Foundation
import MonacoAPI

public struct CabalInvitePreview: Equatable, Sendable {
    public struct Person: Equatable, Sendable {
        public let displayName: String
        public let photoURL: String?
    }

    public struct Proposal: Equatable, Sendable {
        public let id: String
        public let title: String
        public let detail: String
        public let symbol: String
        public let assetName: String
        public let logoURL: String?
        public let yes: Int
        public let no: Int
        public let expiresAt: Date
    }

    public let cabalID: String
    public let name: String
    public let pictureURL: String?
    public let title: String
    public let facts: String
    public let inviter: Person?
    public let inviterLine: String?
    public let faces: [Person]
    public let moreCount: Int
    public let proposal: Proposal?

    public var moreText: String? { moreCount > 0 ? "+\(moreCount)" : nil }
}

public enum CabalInviteCopy {
    public static func preview(
        _ source: Components.Schemas.ReferralCabal, now: Date
    ) -> CabalInvitePreview? {
        guard let cabal = source.cabal else { return nil }
        let faces = cabal.firstMembers.map {
            CabalInvitePreview.Person(displayName: $0.displayName, photoURL: $0.photoUrl)
        }
        let inviter = source.inviter.map {
            CabalInvitePreview.Person(displayName: $0.displayName, photoURL: $0.photoUrl)
        }
        return CabalInvitePreview(
            cabalID: cabal.id, name: cabal.name, pictureURL: cabal.pictureUrl,
            title: title(name: cabal.name),
            facts: facts(memberCount: Int(cabal.memberCount), potMicros: cabal.potValueMicros),
            inviter: inviter, inviterLine: inviter.map { inviterLine(displayName: $0.displayName) },
            faces: faces, moreCount: max(0, Int(cabal.memberCount) - 1 - faces.count),
            proposal: source.openProposal.map { proposal($0, now: now) })
    }

    public static func title(name: String) -> String { "Invite to \(name)" }

    public static func isAdmin(role: String?) -> Bool { role == "creator" || role == "admin" }

    public static func joiningLine(joinMode: String) -> String {
        "Joining: \(joinMode == "open" ? "Open" : "By request")"
    }

    public static func facts(memberCount: Int, potMicros: String) -> String {
        let members = "\(memberCount) \(memberCount == 1 ? "member" : "members")"
        return "\(members) \u{00B7} \(wholeDollars(micros: potMicros)) treasury"
    }

    public static func inviterLine(displayName: String) -> String {
        "\(firstName(displayName)) invited you"
    }

    public static func firstName(_ displayName: String) -> String {
        displayName.split(whereSeparator: \.isWhitespace).first.map(String.init) ?? displayName
    }

    public static func note(
        senderFirstName: String, symbol: String, expiresAt: Date, now: Date, calendar: Calendar
    ) -> String {
        "\(senderFirstName): we vote on \(AssetSymbolFormatter.display(symbol)) "
            + "\(when(expiresAt, now: now, calendar: calendar)). You in?"
    }

    private static func when(_ expiresAt: Date, now: Date, calendar: Calendar) -> String {
        let format = DateFormatter()
        format.calendar = calendar
        format.timeZone = calendar.timeZone
        format.locale = Locale(identifier: "en_US_POSIX")
        let today = calendar.startOfDay(for: now)
        let day = calendar.startOfDay(for: expiresAt)
        let days = calendar.dateComponents([.day], from: today, to: day).day ?? 0
        if days == 0 { return "today" }
        format.dateFormat = days > 0 && days <= 6 ? "EEEE" : "MMM d"
        return format.string(from: expiresAt)
    }

    public static func proposalDetail(
        voted: Int, eligible: Int, replies: Int?, expiresAt: Date, now: Date
    ) -> String {
        ProposalVoteCopy.cardLine(
            cabalName: nil, tally: ProposalTally(yes: voted, no: 0, voters: eligible, needed: 0),
            replies: replies, expiresAt: expiresAt, now: now
        ).short
    }

    private static func proposal(
        _ source: Components.Schemas.ReferralCabal.OpenProposalPayload, now: Date
    ) -> CabalInvitePreview.Proposal {
        let symbol = AssetSymbolFormatter.display(source.symbol)
        let side = source.kind == "sell" ? "Sell" : "Buy"
        return CabalInvitePreview.Proposal(
            id: source.id,
            title: source.usdcMicros.map { "\(side) \(symbol) \u{00B7} \(dollars(micros: $0))" } ?? "\(side) \(symbol)",
            detail: proposalDetail(
                voted: Int(source.voted), eligible: Int(source.eligible), replies: source.replies.map(Int.init),
                expiresAt: source.expiresAt, now: now),
            symbol: source.symbol, assetName: source.assetName, logoURL: source.assetLogoUrl,
            yes: Int(source.yes), no: Int(source.no), expiresAt: source.expiresAt)
    }

    static func wholeDollars(micros raw: String) -> String {
        let micros = Int64(raw) ?? 0
        return trimmed(UsdAmountFormatter.format(micros: micros - micros % 1_000_000))
    }

    private static func dollars(micros raw: String) -> String {
        trimmed(UsdAmountFormatter.format(micros: Int64(raw) ?? 0))
    }

    private static func trimmed(_ amount: String) -> String {
        amount.hasSuffix(".00") ? String(amount.dropLast(3)) : amount
    }
}
