import Foundation

extension Components.Schemas.Cabal {
    public static func sample(role: String?) -> Self {
        sample(role: role, canVote: true)
    }

    public static func sample(role: String?, canVote: Bool) -> Self {
        let creatorID = "01890a5d-ac96-774b-bcce-b302099a8058"
        return Self(
            id: "01890a5d-ac96-774b-bcce-b302099a8060",
            name: "QA pot",
            pictureUrl: nil,
            status: "active",
            rules: .init(
                joinMode: "request", voterMode: "all", threshold: "unanimous", proposalExpirySeconds: 86_400,
                slippageBps: 100, showSides: false, showSidesSince: nil, maxTradeBps: 2000, whoCanPropose: .voters),
            creator: .init(userId: creatorID, handle: "kai", displayName: "Kai", photoUrl: nil),
            memberCount: 2,
            createdAt: Date(timeIntervalSince1970: 1_785_000_000),
            openProposalCount: 1,
            voterCount: 2,
            members: [],
            me: role.map { .init(role: $0, canVote: canVote, canPropose: canVote) },
            myAccessRequest: nil,
            inviteCode: nil,
            treasuryAddress: "treasury-1"
        )
    }

    public static func sampleBanned(role: String?) -> Self {
        var cabal = sample(role: role)
        cabal.status = "banned"
        return cabal
    }

    public static func sampleWithMembers(role: String?) -> Self {
        var cabal = sample(role: role)
        let joined = Date(timeIntervalSince1970: 1_790_000_000)
        cabal.memberCount = 3
        cabal.members = [
            .init(
                userId: cabal.creator.userId, handle: "kai", displayName: "Kai", photoUrl: nil, role: "creator",
                canVote: true, joinedAt: joined),
            .init(
                userId: "01890a5d-ac96-774b-bcce-b302099a8061", handle: "jordan", displayName: "Jordan", photoUrl: nil,
                role: "member", canVote: true, joinedAt: joined),
            .init(
                userId: "01890a5d-ac96-774b-bcce-b302099a8062", handle: "priya", displayName: "Priya", photoUrl: nil,
                role: "member", canVote: true, joinedAt: joined),
        ]
        return cabal
    }
}

extension Components.Schemas.CabalMemberPage {
    public static func sample(members: [Components.Schemas.CabalMember]) -> Self {
        let rows = members.enumerated().map { index, member in
            Components.Schemas.CabalMemberRow(
                userId: member.userId, handle: member.handle, displayName: member.displayName,
                photoUrl: member.photoUrl, role: .init(rawValue: member.role) ?? .member, canVote: member.canVote,
                joinedAt: member.joinedAt, ownershipBps: index == 0 ? 10000 : 0,
                month: .init(right: 0, resolved: 0, voted: 0, eligible: 0), crown: false)
        }
        let admins = rows.filter { $0.role != .member }.count
        return .init(
            items: rows, nextCursor: nil,
            totals: .init(admins: Int32(admins), voters: Int32(rows.count - admins), members: 0))
    }

    public static var sampleACCapital: Self {
        let joined = Date(timeIntervalSince1970: 1_790_000_000)
        let people: [(String, String, String, Components.Schemas.CabalMemberRow.RolePayload, Int32, [Int32])] = [
            ("andres", "Andres", "01890a5d-ac96-774b-bcce-b302099a8071", .admin, 3500, [7, 9, 11, 11]),
            ("christelle", "Christelle", "01890a5d-ac96-774b-bcce-b302099a8072", .admin, 2000, [12, 13, 14, 14]),
            ("logan", "Logan", "01890a5d-ac96-774b-bcce-b302099a8073", .member, 1200, [10, 11, 12, 12]),
            ("mia", "Mia", "01890a5d-ac96-774b-bcce-b302099a8074", .member, 1000, [8, 10, 11, 12]),
            ("jordan", "Jordan", "01890a5d-ac96-774b-bcce-b302099a8075", .member, 800, [6, 8, 9, 10]),
            ("priya", "Priya", "01890a5d-ac96-774b-bcce-b302099a8076", .member, 600, [5, 7, 8, 9]),
            ("sam", "Sam", "01890a5d-ac96-774b-bcce-b302099a8077", .member, 500, [4, 6, 7, 9]),
            ("kai", "Kai", "01890a5d-ac96-774b-bcce-b302099a8078", .member, 400, [11, 14, 14, 15]),
        ]
        let rows = people.map { handle, name, id, role, bps, month in
            Components.Schemas.CabalMemberRow(
                userId: id, handle: handle, displayName: name, photoUrl: nil, role: role, canVote: true,
                joinedAt: joined, ownershipBps: bps,
                month: .init(right: month[0], resolved: month[1], voted: month[2], eligible: month[3]),
                crown: handle == "christelle")
        }
        return .init(items: rows, nextCursor: nil, totals: .init(admins: 2, voters: 6, members: 0))
    }
}
