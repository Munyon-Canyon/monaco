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

    public static func proposalDetail(
        voted: Int, eligible: Int, replies: Int?, expiresAt: Date, now: Date
    ) -> String {
        var parts = ["\(voted) of \(eligible) voted"]
        if let replies { parts.append("\(replies) \(replies == 1 ? "reply" : "replies")") }
        parts.append(endsIn(expiresAt, now: now))
        return parts.joined(separator: " \u{00B7} ")
    }

    public static func endsIn(_ date: Date, now: Date) -> String {
        let seconds = date.timeIntervalSince(now)
        guard seconds >= 60 else { return seconds > 0 ? "ends in 1m" : "ends now" }
        let minutes = Int(seconds / 60)
        if minutes < 60 { return "ends in \(minutes)m" }
        let hours = minutes / 60
        return hours < 24 ? "ends in \(hours)h" : "ends in \(hours / 24)d"
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
            yes: Int(source.yes), no: Int(source.no))
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
