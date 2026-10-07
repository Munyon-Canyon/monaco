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
                slippageBps: 100),
            creator: .init(userId: creatorID, handle: "kai", displayName: "Kai", photoUrl: nil),
            memberCount: 2,
            members: [],
            me: role.map { .init(role: $0, canVote: canVote) },
            myAccessRequest: nil,
            inviteCode: role == nil ? nil : "ABCD2345",
            treasuryAddress: "treasury-1"
        )
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
