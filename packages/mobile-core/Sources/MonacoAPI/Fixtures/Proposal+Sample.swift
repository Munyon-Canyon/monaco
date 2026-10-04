import Foundation

extension Components.Schemas.ProposalDetail {
    public static let sampleProposerID = "01890a5d-ac96-774b-bcce-b302099a8061"
    public static let sampleVoterIDs = [
        "01890a5d-ac96-774b-bcce-b302099a8058",
        "01890a5d-ac96-774b-bcce-b302099a8061",
        "01890a5d-ac96-774b-bcce-b302099a8062",
    ]

    public static func sample(
        id: String = "01890a5d-ac96-774b-bcce-b302099a8057",
        cabalID: String = Components.Schemas.Cabal.sample(role: "member").id,
        kind: Components.Schemas.ProposalKind = .buy,
        symbol: String = "GOOGLx",
        status: Components.Schemas.ProposalStatus = .open,
        statusMessage: String? = nil,
        thesis: String? = "Cloud keeps growing and the stock is cheap next to its peers.",
        ballots: [String: Components.Schemas.BallotChoice] = [:],
        viewerID: String = sampleVoterIDs[0],
        canVote: Bool = true,
        swap: SwapPayload? = nil,
        now: Date
    ) -> Self {
        let voters = sampleVoterIDs.map { userID in
            Components.Schemas.ProposalVoter(
                userId: userID,
                choice: ballots[userID].flatMap { .init(rawValue: $0.rawValue) },
                castAt: ballots[userID] == nil ? nil : now.addingTimeInterval(-600))
        }
        let yes = ballots.values.filter { $0 == .yes }.count
        return Self(
            id: id,
            cabalId: cabalID,
            proposerId: sampleProposerID,
            kind: kind,
            symbol: symbol,
            usdcMicros: kind == .buy ? 250_000_000 : nil,
            tokenAmount: kind == .sell ? 60_170_000 : nil,
            quoteOutAmount: kind == .buy ? 73_191_440 : 139_000_000,
            thesis: thesis,
            status: status,
            statusReason: statusMessage == nil ? nil : "pot_short",
            statusMessage: statusMessage,
            expiresAt: now.addingTimeInterval(23 * 3600 + 1800),
            createdAt: now.addingTimeInterval(-33 * 60),
            tally: .init(yes: yes, no: ballots.count - yes, voters: voters.count, needed: 2),
            myBallot: ballots[viewerID].flatMap { .init(rawValue: $0.rawValue) },
            voters: voters,
            canVote: canVote && status == .open,
            canWithdraw: false,
            swap: swap
        )
    }

    public var summary: Components.Schemas.Proposal {
        .init(
            id: id, cabalId: cabalId, proposerId: proposerId, kind: kind, symbol: symbol, usdcMicros: usdcMicros,
            tokenAmount: tokenAmount, quoteOutAmount: quoteOutAmount, thesis: thesis, status: status,
            statusReason: statusReason, statusMessage: statusMessage, expiresAt: expiresAt, createdAt: createdAt,
            tally: tally, myBallot: myBallot.flatMap { .init(rawValue: $0.rawValue) })
    }
}

extension Components.Schemas.ProposalDetail.SwapPayload {
    public static func failed(retryable: Bool) -> Self {
        .init(
            swapId: "01890a5d-ac96-774b-bcce-b302099a8063",
            status: .failed,
            failureCode: "slippage_exceeded",
            failureMessage: "The price moved too far before the trade went through.",
            txSignature: nil,
            retryable: retryable
        )
    }
}

extension Components.Schemas.AssetDetail {
    public static func proposalSample(symbol: String = "GOOGLx", kind: Components.Schemas.AssetKind = .equity) -> Self {
        .init(
            symbol: symbol,
            displayName: kind == .preIpo ? "SpaceX" : "Alphabet",
            issuer: kind == .preIpo ? .tessera : .xstocks,
            kind: kind,
            logoUrl: nil,
            priceMicros: 341_570_000,
            priceAsOf: nil,
            changeBps: nil,
            sparklineMicros: nil,
            session: .init(state: .open, continuous: kind == .preIpo, holiday: "", earlyClose: false),
            decimals: 8,
            uiMultiplier: .init(num: 1, den: 1),
            tradable: true,
            otherListings: [],
            attribution: "Data provided by CoinGecko"
        )
    }
}
