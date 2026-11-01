import MonacoAPI

public enum ProfileReturnTone: Equatable, Sendable {
    case gain
    case loss
    case flat
}

public enum ProfileCabalRow: Equatable, Sendable, Identifiable {
    public struct Member: Equatable, Sendable {
        public let id: String
        public let name: String
        public let pictureURL: String?
        public let role: String
        public var ownership: String?
        public var value: String?
        public var valueMicros: Int64?
        public var returnText: String?
        public var returnTone: ProfileReturnTone?
        public var note: StakeValueNote?

        public var subtitle: String {
            ownership.map { "\($0) · \(role)" } ?? role
        }
    }

    public struct Invited: Equatable, Sendable {
        public let requestID: String
        public let cabalID: String
        public let name: String
        public let pictureURL: String?
        public let subtitle: String
    }

    case member(Member)
    case invited(Invited)

    public var id: String {
        switch self {
        case .member(let member): member.id
        case .invited(let invite): "invite-\(invite.requestID)"
        }
    }
}

public enum ProfileCabalRows {
    public static func make(
        cabals: [Components.Schemas.MyCabal],
        portfolio: Components.Schemas.MyPortfolio?,
        invites: [Components.Schemas.CabalInvite]
    ) -> [ProfileCabalRow] {
        let held = Dictionary(
            (portfolio?.cabals ?? []).map { ($0.cabal.id, $0) }, uniquingKeysWith: { first, _ in first })
        let members = cabals.map { member($0, stake: held[$0.id], hasPortfolio: portfolio != nil) }
        let valued = members.filter { $0.valueMicros != nil }.sorted {
            $0.valueMicros == $1.valueMicros ? $0.name < $1.name : ($0.valueMicros ?? 0) > ($1.valueMicros ?? 0)
        }
        let rest = members.filter { $0.valueMicros == nil }.sorted { $0.name < $1.name }
        return (valued + rest).map(ProfileCabalRow.member) + invites.map(invited)
    }

    public static func memberCount(_ rows: [ProfileCabalRow]) -> Int {
        rows.filter { if case .member = $0 { true } else { false } }.count
    }

    static func role(_ cabal: Components.Schemas.MyCabal) -> String {
        switch cabal.role {
        case "admin", "creator": "admin"
        default: cabal.canVote ? "voter" : "member"
        }
    }

    static func ownership(bps: Int32) -> String {
        let basisPoints = Int64(bps)
        guard basisPoints < 1000 else {
            return PercentFormatter.format(basisPoints: basisPoints, signed: false, fractionDigits: 0)
        }
        return PercentFormatter.format(basisPoints: basisPoints, signed: false, fractionDigits: 1)
            .replacingOccurrences(of: ".0%", with: "%")
    }

    private static func member(
        _ cabal: Components.Schemas.MyCabal, stake: Components.Schemas.PortfolioCabal?, hasPortfolio: Bool
    ) -> ProfileCabalRow.Member {
        var row = ProfileCabalRow.Member(
            id: cabal.id, name: cabal.name, pictureURL: cabal.pictureUrl, role: role(cabal))
        guard hasPortfolio else { return row }
        let arriving = stake?.arrivingMicros ?? 0
        guard let stake, stake.shareUnits > 0 else {
            if arriving > 0 {
                row.value = UsdAmountFormatter.format(flooredMicros: arriving)
                row.valueMicros = arriving
                row.note = .settling
                return row
            }
            let snapshotted = (stake?.potValueMicros ?? 0) > 0
            row.value = snapshotted ? UsdAmountFormatter.format(micros: 0) : "—"
            return row
        }
        let worth = max(0, stake.valueMicros + arriving)
        row.ownership = ownership(bps: stake.ownershipBps)
        row.value = UsdAmountFormatter.format(flooredMicros: worth)
        row.valueMicros = worth
        if let bps = stake.returnBps {
            row.returnText = PercentFormatter.format(basisPoints: bps, signed: true, fractionDigits: 1)
            row.returnTone = bps > 0 ? .gain : (bps < 0 ? .loss : .flat)
        } else {
            row.returnText = "—"
            row.returnTone = .flat
        }
        row.note = StakeValueNote.make(
            valueMicros: stake.valueMicros, arrivingMicros: arriving, isStale: stake.stale ?? false,
            valuedAt: stake.valuedAt)
        return row
    }

    private static func invited(_ invite: Components.Schemas.CabalInvite) -> ProfileCabalRow {
        let inviter = invite.invitedBy
        let who =
            inviter.displayName.isEmpty
            ? (inviter.handle.map { "@\($0)" } ?? "Someone") : inviter.displayName
        let count = invite.cabal.memberCount
        return .invited(
            .init(
                requestID: invite.requestId, cabalID: invite.cabal.id, name: invite.cabal.name,
                pictureURL: invite.cabal.pictureUrl,
                subtitle: "Invited by \(who) · \(count) \(count == 1 ? "member" : "members")"))
    }
}
