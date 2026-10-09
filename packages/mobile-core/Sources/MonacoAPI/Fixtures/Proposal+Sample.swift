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
    public static func sample(
        status: Components.Schemas.ProposalStatus = .open, kind: Components.Schemas.ProposalKind = .buy,
        myBallot: Components.Schemas.BallotChoice? = nil, canVote: Bool = false, swap: SwapPayload? = nil
    ) -> Self {
        let members = Components.Schemas.Cabal.sampleWithMembers(role: "member").members
        let isOpen = status == .open
        let isSell = kind == .sell
        let choices: [Components.Schemas.BallotChoice?] = [
            .yes, isOpen || status == .expired ? nil : .yes, myBallot,
        ]
        let castOffsets: [TimeInterval] = [-6_000, -5_000, -4_000]
        let voters = members.prefix(3).enumerated().map { index, member in
            Components.Schemas.ProposalVoter(
                userId: member.userId,
                choice: choices[index]
                    .flatMap { Components.Schemas.ProposalVoter.ChoicePayload(rawValue: $0.rawValue) },
                castAt: choices[index] == nil ? nil : .now.addingTimeInterval(castOffsets[index]))
        }
        return .init(
            id: "proposal-1", cabalId: Components.Schemas.Cabal.sample(role: nil).id, proposerId: members[1].userId,
            kind: kind, symbol: "GOOGLx", usdcMicros: isSell ? nil : 25_000_000,
            tokenAmount: isSell ? 150_000_000 : nil,
            quoteOutAmount: isSell ? 25_000_000 : 14_250_000, thesis: "Earnings next week.", status: status,
            statusReason: nil, statusMessage: nil,
            expiresAt: .now.addingTimeInterval(isOpen ? 43_200 : -3_600),
            createdAt: .now.addingTimeInterval(-7_200),
            tally: .init(
                yes: choices.filter { $0 == .yes }.count, no: choices.filter { $0 == .no }.count, voters: 3,
                needed: isOpen ? 3 : 2),
            myBallot: myBallot.flatMap { MyBallotPayload(rawValue: $0.rawValue) }, voters: voters,
            canVote: canVote, canWithdraw: false, swap: swap)
    }

    public static func failedSwap(
        retryable: Bool, failureCode: String = "jupiter_failed",
        failureMessage: String = "The trade did not go through."
    ) -> Self {
        sample(
            status: .passed,
            swap: .init(
                swapId: "swap-1", status: .failed, failureCode: failureCode,
                failureMessage: failureMessage, txSignature: nil, retryable: retryable))
    }
}

extension Components.Schemas.PendingVote {
    public static let samples: [Self] = [
        .init(
            proposalId: "proposal-1", cabalId: Components.Schemas.Cabal.sample(role: nil).id, kind: .buy,
            symbol: "GOOGLx", expiresAt: .now.addingTimeInterval(43_200)),
        .init(
            proposalId: "proposal-2", cabalId: Components.Schemas.Cabal.sample(role: nil).id, kind: .buy,
            symbol: "GOOGLx", expiresAt: .now.addingTimeInterval(86_400)),
    ]
}

extension Components.Schemas.ProposalDetail {
    public static var priceMoved: Self {
        failedSwap(
            retryable: true, failureCode: "price_moved",
            failureMessage: "The price moved past the cabal's limit since the vote.")
    }
}
