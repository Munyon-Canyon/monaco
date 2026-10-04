import Foundation
import MonacoAPI

public struct ProposalSwap: Equatable, Sendable {
    public let status: String
    public let retryable: Bool

    init(_ swap: Components.Schemas.ProposalDetail.SwapPayload?) {
        status = swap?.status.rawValue ?? ""
        retryable = swap?.retryable ?? false
    }
}

public struct ProposalSummary: Identifiable, Equatable, Sendable {
    public let id: String
    public let cabalID: String
    public let kind: String
    public let symbol: String
    public let status: ProposalStatus
    public let statusMessage: String?
    public let expiresAt: Date
    public let canVote: Bool
    public let canWithdraw: Bool
    public let swap: ProposalSwap?

    init(
        _ proposal: Components.Schemas.Proposal,
        canVote: Bool = false,
        canWithdraw: Bool = false,
        swap: ProposalSwap? = nil
    ) {
        id = proposal.id
        cabalID = proposal.cabalId
        kind = proposal.kind.rawValue
        symbol = proposal.symbol
        status = ProposalStatus(proposal.status)
        statusMessage = proposal.statusMessage
        expiresAt = proposal.expiresAt
        self.canVote = canVote
        self.canWithdraw = canWithdraw
        self.swap = swap
    }
}

public struct ProposalDetail: Identifiable, Equatable, Sendable {
    public let summary: ProposalSummary
    public let voterIDs: [String]
    public var id: String { summary.id }
}

public struct PendingVote: Identifiable, Equatable, Sendable {
    public let id: String
    public let cabalID: String
    public let kind: String
    public let symbol: String
    public let expiresAt: Date

    init(_ value: Components.Schemas.PendingVote) {
        id = value.proposalId
        cabalID = value.cabalId
        kind = value.kind.rawValue
        symbol = value.symbol
        expiresAt = value.expiresAt
    }
}

public enum ProposalFilter: String, Sendable { case open, closed }

public struct ProposalsRepository: Sendable {
    private let api: APIClient

    public init(api: APIClient) { self.api = api }

    public func list(
        cabalID: String,
        filter: ProposalFilter,
        cursor: String?
    ) async throws -> (items: [ProposalSummary], nextCursor: String?) {
        try await api.read { client in
            let response = try await client.getCabalProposals(
                path: .init(id: cabalID),
                query: .init(filter: .init(rawValue: filter.rawValue), cursor: cursor)
            ).ok.body.json
            return (response.proposals.map { ProposalSummary($0) }, response.nextCursor)
        }
    }

    public func detail(id: String) async throws -> ProposalDetail {
        try await api.read { client in
            let value = try await client.getProposal(path: .init(id: id)).ok.body.json
            let summary = ProposalSummary(
                .init(
                    id: value.id,
                    cabalId: value.cabalId,
                    proposerId: value.proposerId,
                    kind: value.kind,
                    symbol: value.symbol,
                    usdcMicros: value.usdcMicros,
                    tokenAmount: value.tokenAmount,
                    quoteOutAmount: value.quoteOutAmount,
                    thesis: value.thesis,
                    status: value.status,
                    statusReason: value.statusReason,
                    statusMessage: value.statusMessage,
                    expiresAt: value.expiresAt,
                    createdAt: value.createdAt,
                    tally: value.tally
                ),
                canVote: value.canVote,
                canWithdraw: value.canWithdraw,
                swap: ProposalSwap(value.swap)
            )
            return ProposalDetail(summary: summary, voterIDs: value.voters.map(\.userId))
        }
    }

    public func vote(id: String, choice: String, submission: IdempotentSubmission) async throws {
        let body = Components.Schemas.CastVoteRequest(choice: .init(rawValue: choice) ?? .yes)
        try await api.submit(submission, payload: body, operation: "postProposalVote") { client, key in
            _ = try await client.postProposalVote(
                path: .init(id: id),
                headers: .init(idempotencyKey: key),
                body: .json(body)
            ).ok.body.json
        }
    }

    public func pendingVotes() async throws -> [PendingVote] {
        try await api.read { client in
            try await client.getMyPendingVotes().ok.body.json.map(PendingVote.init)
        }
    }
}
