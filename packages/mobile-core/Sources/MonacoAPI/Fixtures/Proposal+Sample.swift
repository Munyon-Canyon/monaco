import Foundation

extension Components.Schemas.Proposal {
    public static func sample(
        status: Components.Schemas.ProposalStatus = .open, kind: Components.Schemas.ProposalKind = .buy
    ) -> Self {
        .init(
            id: "proposal-1", cabalId: "cabal-1", cabalName: "Friday Night Lights", proposerId: "user-1", kind: kind,
            symbol: "AAPLx", usdcMicros: 1_000_000,
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
            id: "proposal-1", cabalId: Components.Schemas.Cabal.sample(role: nil).id,
            cabalName: Components.Schemas.Cabal.sample(role: nil).name,
            stats: .init(
                treasuryShareBps: 250, closesAt: .now.addingTimeInterval(isOpen ? 43_200 : -3_600),
                closedAt: isOpen ? nil : .now.addingTimeInterval(-3_600)),
            replyCount: 6, proposerId: members[1].userId,
            proposerReturn1mBps: 820, proposerReturnPrivate: false, kind: kind,
            symbol: "GOOGLx", usdcMicros: isSell ? nil : 25_000_000,
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
                failureMessage: failureMessage, txSignature: nil, retryable: retryable, tokenDecimals: 8))
    }
}

extension Components.Schemas.PendingVote {
    public static func sample(
        proposalId: String = "proposal-nvda", cabalId: String = Components.Schemas.Cabal.sample(role: nil).id,
        cabalName: String = "AC Capital", kind: Components.Schemas.ProposalKind = .buy, symbol: String = "NVDAx",
        assetName: String = "NVIDIA", expiresAt: Date = .now.addingTimeInterval(14_400)
    ) -> Self {
        .init(
            proposalId: proposalId, cabalId: cabalId, cabalName: cabalName, kind: kind, symbol: symbol,
            usdcMicros: kind == .sell ? nil : 400_000_000, tokenAmount: kind == .sell ? 1_820_000 : nil,
            assetName: assetName, assetLogoUrl: nil,
            proposer: .init(userId: "user-mia", handle: "mia", displayName: "Mia", photoUrl: nil),
            tally: .init(yes: 4, no: 1, voters: 8, needed: 5), votedCount: 5, replyCount: 6,
            createdAt: expiresAt.addingTimeInterval(-21_600), expiresAt: expiresAt)
    }

    public static let nvda: Self = .sample()

    public static let samples: [Self] = [
        .sample(
            proposalId: "proposal-1", cabalName: Components.Schemas.Cabal.sample(role: nil).name,
            symbol: "GOOGLx", assetName: "Alphabet", expiresAt: .now.addingTimeInterval(43_200)),
        .sample(
            proposalId: "proposal-2", cabalName: Components.Schemas.Cabal.sample(role: nil).name,
            symbol: "GOOGLx", assetName: "Alphabet", expiresAt: .now.addingTimeInterval(86_400)),
    ]
}

extension Components.Schemas.ProposalDetail {
    public static var priceMoved: Self {
        failedSwap(
            retryable: true, failureCode: "price_moved",
            failureMessage: "The price moved past the cabal's limit since the vote.")
    }
}

extension Components.Schemas.ProposalDetail {
    public static func openNVDA(now: Date = .now) -> Self {
        trade(
            symbol: "NVDAx", kind: .buy, usdcMicros: 400_000_000, tokenAmount: nil, quoteOut: 182_000_000,
            status: .open, now: now, closesIn: 14_400, closedAgo: nil,
            stats: .init(
                treasuryShareBps: 200, closesAt: now.addingTimeInterval(14_400), closedAt: nil,
                priceMicros: 220_000_000, sharesAtomics: 182_000_000, sharesExact: false, tokenDecimals: 8),
            swap: nil)
    }

    public static func passedNVDA(now: Date = .now) -> Self {
        trade(
            symbol: "NVDAx", kind: .buy, usdcMicros: 400_000_000, tokenAmount: nil, quoteOut: 182_000_000,
            status: .passed, now: now, closesIn: 14_400, closedAgo: 720,
            stats: .init(
                treasuryShareBps: 200, closesAt: now.addingTimeInterval(14_400),
                closedAt: now.addingTimeInterval(-720), priceMicros: 220_000_000, sharesAtomics: 182_000_000,
                sharesExact: false, tokenDecimals: 8),
            swap: nil)
    }

    public static func filledNVDA(now: Date = .now) -> Self {
        trade(
            symbol: "NVDAx", kind: .buy, usdcMicros: 400_000_000, tokenAmount: nil, quoteOut: 182_000_000,
            status: .executed, now: now, closesIn: 14_400, closedAgo: 720,
            stats: .init(
                treasuryShareBps: 200, closesAt: now.addingTimeInterval(14_400),
                closedAt: now.addingTimeInterval(-720), priceMicros: 220_000_000, sharesAtomics: 182_000_000,
                sharesExact: true, tokenDecimals: 8, sinceBps: 30),
            swap: .init(
                swapId: "swap-nvda", status: .confirmed, failureCode: nil, failureMessage: nil,
                txSignature: "sig-nvda", retryable: false, usdcMicros: 399_308_000, tokenAmount: 182_000_000,
                tokenDecimals: 8, priceMicros: 219_400_000, feeMicros: 400_000,
                filledAt: now.addingTimeInterval(-720)))
    }

    public static func filledMETA(now: Date = .now) -> Self {
        trade(
            symbol: "METAx", kind: .buy, usdcMicros: 300_000_000, tokenAmount: nil, quoteOut: 42_000_000,
            status: .executed, now: now, closesIn: -18_000, closedAgo: 18_000,
            stats: .init(
                treasuryShareBps: 128, closesAt: now.addingTimeInterval(-18_000),
                closedAt: now.addingTimeInterval(-18_000), priceMicros: 724_400_000, sharesAtomics: 42_000_000,
                sharesExact: true, tokenDecimals: 8, sinceBps: 110),
            swap: .init(
                swapId: "swap-meta", status: .confirmed, failureCode: nil, failureMessage: nil,
                txSignature: "sig-meta", retryable: false, usdcMicros: 300_000_000, tokenAmount: 42_000_000,
                tokenDecimals: 8, priceMicros: 716_520_000, feeMicros: 300_000,
                filledAt: now.addingTimeInterval(-17_940)))
    }

    public static func failedTSLA(now: Date = .now) -> Self {
        let closed = now.addingTimeInterval(-(3 * 86_400 + 15 * 3_600 + 53 * 60))
        return trade(
            symbol: "TSLAx", kind: .sell, usdcMicros: nil, tokenAmount: 220_000_000, quoteOut: 900_000_000,
            status: .failed, now: now, closesIn: -400_000, closedAgo: 400_000,
            stats: .init(
                treasuryShareBps: 398, closesAt: closed, closedAt: closed, priceMicros: 412_550_000,
                sharesAtomics: 220_000_000, sharesExact: true, tokenDecimals: 8),
            swap: nil)
    }

    public static func preIpoSpaceX(now: Date = .now) -> Self {
        trade(
            symbol: "tSpaceX", kind: .buy, usdcMicros: 500_000_000, tokenAmount: nil, quoteOut: 270_000_000,
            status: .open, now: now, closesIn: 72_000, closedAgo: nil,
            stats: .init(
                treasuryShareBps: 220, closesAt: now.addingTimeInterval(72_000), closedAt: nil,
                priceMicros: 185_000_000, sharesAtomics: 270_000_000, sharesExact: false, tokenDecimals: 8),
            swap: nil)
    }

    private static func trade(
        symbol: String, kind: Components.Schemas.ProposalKind, usdcMicros: Int64?, tokenAmount: Int64?,
        quoteOut: Int64, status: Components.Schemas.ProposalStatus, now: Date, closesIn: TimeInterval,
        closedAgo: TimeInterval?, stats: Components.Schemas.ProposalStats, swap: SwapPayload?
    ) -> Self {
        var detail = sample(status: status, kind: kind, swap: swap)
        detail.stats = stats
        detail.symbol = symbol
        detail.usdcMicros = usdcMicros
        detail.tokenAmount = tokenAmount
        detail.quoteOutAmount = quoteOut
        detail.createdAt = now.addingTimeInterval(-7_200)
        detail.expiresAt = now.addingTimeInterval(closesIn)
        detail.myVote = nil
        detail.replyCount = 6
        return detail
    }
}
