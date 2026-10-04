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
                joinMode: "open", voterMode: "all", threshold: "unanimous", proposalExpirySeconds: 86_400,
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
}
