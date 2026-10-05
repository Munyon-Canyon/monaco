import Foundation
import MonacoAPI

public struct ProposalSwap: Equatable, Sendable {
    public let status: String
    public let id: String?
    public let failureMessage: String?
    public let retryable: Bool

    init(_ swap: Components.Schemas.ProposalDetail.SwapPayload?) {
        status = swap?.status.rawValue ?? ""
        id = swap?.swapId
        failureMessage = swap?.failureMessage
        retryable = swap?.retryable ?? false
    }
}

public struct ProposalSummary: Identifiable, Equatable, Sendable {
    public let id: String
    public let cabalID: String
    public let kind: String
    public let symbol: String
    public let proposerID: String
    public let thesis: String?
    public let usdcMicros: Int64?
    public let tokenAmount: Int64?
    public let quoteOutAmount: Int64
    public let createdAt: Date
    public let tally: ProposalTally
    public let myBallot: String?
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
        proposerID = proposal.proposerId
        thesis = proposal.thesis
        usdcMicros = proposal.usdcMicros
        tokenAmount = proposal.tokenAmount
        quoteOutAmount = proposal.quoteOutAmount
        createdAt = proposal.createdAt
        tally = ProposalTally(proposal.tally)
        myBallot = proposal.myBallot?.rawValue
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
    public let voters: [ProposalVoter]
    public var id: String { summary.id }

    init(summary: ProposalSummary, voters: [ProposalVoter]) {
        self.summary = summary
        self.voters = voters
    }

    init(_ value: Components.Schemas.ProposalDetail) {
        let proposal = Components.Schemas.Proposal(
            id: value.id, cabalId: value.cabalId, proposerId: value.proposerId, kind: value.kind,
            symbol: value.symbol, usdcMicros: value.usdcMicros, tokenAmount: value.tokenAmount,
            quoteOutAmount: value.quoteOutAmount, thesis: value.thesis, status: value.status,
            statusReason: value.statusReason, statusMessage: value.statusMessage, expiresAt: value.expiresAt,
            createdAt: value.createdAt, tally: value.tally)
        self.init(
            summary: ProposalSummary(
                proposal, canVote: value.canVote, canWithdraw: value.canWithdraw, swap: ProposalSwap(value.swap)),
            voters: value.voters.map(ProposalVoter.init))
    }
}

public struct ProposalTally: Equatable, Sendable {
    public let yes: Int
    public let no: Int
    public let voters: Int
    public let needed: Int

    init(_ value: Components.Schemas.Tally) {
        yes = value.yes
        no = value.no
        voters = value.voters
        needed = value.needed
    }
}

public struct ProposalVoter: Equatable, Sendable, Identifiable {
    public let id: String
    public let ballot: String?

    init(_ value: Components.Schemas.ProposalVoter) {
        id = value.userId
        ballot = value.choice?.rawValue
    }
}

public struct ProposalMember: Equatable, Sendable, Identifiable {
    public let id: String
    public let name: String
    public let photoURL: URL?

    init(_ value: Components.Schemas.CabalMember) {
        id = value.userId
        name = value.displayName
        photoURL = value.photoUrl.flatMap(URL.init(string:))
    }
}

public struct ProposalAsset: Equatable, Sendable {
    public let displayName: String
    public let kind: AssetKind
    public let decimals: Int
    public let logoURL: URL?

    init(_ value: Components.Schemas.AssetDetail) {
        displayName = value.displayName
        kind = AssetKind(raw: value.kind.rawValue)
        decimals = value.decimals
        logoURL = value.logoUrl.flatMap(URL.init(string:))
    }
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
    let api: APIClient

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
            return ProposalDetail(value)
        }
    }

    public func isPaused(cabalID: String) async throws -> Bool {
        try await api.read { client in
            try await client.getCashOutPreview(path: .init(id: cabalID)).ok.body.json.pause != nil
        }
    }

    public func members(cabalID: String) async throws -> [ProposalMember] {
        try await api.read { client in
            try await client.getCabal(path: .init(id: cabalID)).ok.body.json.members.map(ProposalMember.init)
        }
    }

    public func asset(symbol: String) async throws -> ProposalAsset {
        try await api.read { client in
            ProposalAsset(try await client.getAsset(path: .init(symbol: symbol)).ok.body.json)
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
