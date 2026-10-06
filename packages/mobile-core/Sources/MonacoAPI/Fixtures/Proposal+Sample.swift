import Foundation

extension Components.Schemas.Proposal {
    public static func sample(
        status: Components.Schemas.ProposalStatus = .open, kind: Components.Schemas.ProposalKind = .buy
    ) -> Self {
        .init(
            id: "proposal-1", cabalId: "cabal-1", proposerId: "user-1", kind: kind, symbol: "AAPLx",
            usdcMicros: 1_000_000,
            quoteOutAmount: 1, status: status, expiresAt: .now.addingTimeInterval(86_400), createdAt: .now,
            tally: .init(yes: 0, no: 0, voters: 3, needed: 2), canVote: false)
    }
}

extension Components.Schemas.ProposalDetail {
    public static func failedSwap(retryable: Bool) -> Self {
        let members = Components.Schemas.Cabal.sampleWithMembers(role: "member").members
        return .init(
            id: "proposal-1", cabalId: "cabal-1", proposerId: members[1].userId, kind: .buy, symbol: "GOOGLx",
            usdcMicros: 25_000_000, tokenAmount: nil, quoteOutAmount: 14_250_000, thesis: "Earnings next week.",
            status: .passed, statusReason: nil, statusMessage: nil, expiresAt: .now.addingTimeInterval(-3_600),
            createdAt: .now.addingTimeInterval(-7_200), tally: .init(yes: 2, no: 0, voters: 3, needed: 2),
            myBallot: nil,
            voters: [
                .init(userId: members[0].userId, choice: .yes, castAt: .now.addingTimeInterval(-6_000)),
                .init(userId: members[1].userId, choice: .yes, castAt: .now.addingTimeInterval(-5_000)),
                .init(userId: members[2].userId, choice: nil, castAt: nil),
            ], canVote: false, canWithdraw: false,
            swap: .init(
                swapId: "swap-1", status: .failed, failureCode: "jupiter_failed",
                failureMessage: "The trade did not go through.", txSignature: nil, retryable: retryable))
    }
}
