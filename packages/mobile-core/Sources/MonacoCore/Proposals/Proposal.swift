import Foundation
import MonacoAPI

public struct ProposalTally: Equatable, Sendable {
    public let yes: Int
    public let no: Int
    public let voters: Int
    public let needed: Int

    public var voted: Int { yes + no }

    init(_ tally: Components.Schemas.Tally) {
        yes = tally.yes
        no = tally.no
        voters = tally.voters
        needed = tally.needed
    }
}

public struct ProposalTrade: Equatable, Sendable {
    public let swapID: String
    public let state: SwapState
    public let failureMessage: String?
    public let retryable: Bool

    init(_ swap: Components.Schemas.ProposalDetail.SwapPayload) {
        swapID = swap.swapId
        state =
            switch swap.status {
            case .created: .created
            case .submitted: .submitted
            case .confirmed: .confirmed
            case .failed: .failed
            }
        failureMessage = swap.failureMessage
        retryable = swap.retryable
    }
}

public struct Proposal: Identifiable, Equatable, Sendable {
    public let id: String
    public let cabalID: String
    public let proposerID: String
    public let kind: ProposalKind
    public let symbol: String
    public let usdcMicros: Int64?
    public let tokenAmount: Int64?
    public let quoteOutAmount: Int64
    public let thesis: String?
    public let status: ProposalStatus
    public let statusMessage: String?
    public let expiresAt: Date
    public let createdAt: Date
    public let tally: ProposalTally
    public let myBallot: BallotChoice?

    init(_ proposal: Components.Schemas.Proposal) {
        id = proposal.id
        cabalID = proposal.cabalId
        proposerID = proposal.proposerId
        kind = ProposalKind(proposal.kind)
        symbol = proposal.symbol
        usdcMicros = proposal.usdcMicros
        tokenAmount = proposal.tokenAmount
        quoteOutAmount = proposal.quoteOutAmount
        thesis = Self.reason(proposal.thesis)
        status = ProposalStatus(proposal.status)
        statusMessage = proposal.statusMessage
        expiresAt = proposal.expiresAt
        createdAt = proposal.createdAt
        tally = ProposalTally(proposal.tally)
        myBallot = proposal.myBallot.flatMap { BallotChoice(rawValue: $0.rawValue) }
    }

    init(_ detail: Components.Schemas.ProposalDetail) {
        id = detail.id
        cabalID = detail.cabalId
        proposerID = detail.proposerId
        kind = ProposalKind(detail.kind)
        symbol = detail.symbol
        usdcMicros = detail.usdcMicros
        tokenAmount = detail.tokenAmount
        quoteOutAmount = detail.quoteOutAmount
        thesis = Self.reason(detail.thesis)
        status = ProposalStatus(detail.status)
        statusMessage = detail.statusMessage
        expiresAt = detail.expiresAt
        createdAt = detail.createdAt
        tally = ProposalTally(detail.tally)
        myBallot = detail.myBallot.flatMap { BallotChoice(rawValue: $0.rawValue) }
    }

    private static func reason(_ thesis: String?) -> String? {
        guard let trimmed = thesis?.trimmingCharacters(in: .whitespacesAndNewlines), !trimmed.isEmpty else {
            return nil
        }
        return trimmed
    }
}

public struct ProposalVoter: Identifiable, Equatable, Sendable {
    public let userID: String
    public let choice: BallotChoice?

    public var id: String { userID }
}

public struct ProposalDetail: Identifiable, Equatable, Sendable {
    public let proposal: Proposal
    public let voters: [ProposalVoter]
    public let canVote: Bool
    public let canWithdraw: Bool
    public let trade: ProposalTrade?

    public var id: String { proposal.id }

    public var steps: ProposalSteps {
        let message = proposal.status == .executionBlocked ? proposal.statusMessage : trade?.failureMessage
        return ProposalSteps(status: proposal.status, kind: proposal.kind, swap: trade?.state, failureMessage: message)
    }

    init(_ detail: Components.Schemas.ProposalDetail) {
        proposal = Proposal(detail)
        voters = detail.voters.map {
            ProposalVoter(userID: $0.userId, choice: $0.choice.flatMap { BallotChoice(rawValue: $0.rawValue) })
        }
        canVote = detail.canVote
        canWithdraw = detail.canWithdraw
        trade = detail.swap.map(ProposalTrade.init)
    }
}
