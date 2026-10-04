import Foundation
import MonacoAPI

public enum ProposalFilter: String, CaseIterable, Sendable {
    case open
    case closed
}

public struct PendingVote: Identifiable, Equatable, Sendable {
    public let id: String
    public let cabalID: String
}

public struct ProposalLookups: Sendable {
    let cabals: [String: ProposalCabal]
    let assets: [String: ProposalAsset]

    public func cabal(_ id: String) -> ProposalCabal? { cabals[id] }

    public func card(_ proposal: Proposal, canVote: Bool? = nil, swap: SwapState? = nil) -> ProposalCard {
        let cabal = cabals[proposal.cabalID]
        return ProposalCard(
            proposal: proposal,
            asset: assets[proposal.symbol] ?? ProposalAsset(unlisted: proposal.symbol),
            proposer: cabal?.person(proposal.proposerID) ?? ProposalPerson(unknown: proposal.proposerID),
            canVote: canVote ?? cabal?.canVote ?? false,
            swap: swap
        )
    }
}

public struct ProposalsRepository: Sendable {
    private let api: APIClient

    public init(api: APIClient) { self.api = api }

    public func list(
        cabalID: String,
        filter: ProposalFilter,
        cursor: String?
    ) async throws -> (items: [Proposal], nextCursor: String?) {
        let wire: Operations.GetCabalProposals.Input.Query.FilterPayload =
            switch filter {
            case .open: .open
            case .closed: .closed
            }
        return try await api.read { client in
            let page = try await client.getCabalProposals(
                path: .init(id: cabalID),
                query: .init(filter: wire, cursor: cursor)
            ).ok.body.json
            return (page.proposals.map(Proposal.init), page.nextCursor)
        }
    }

    public func detail(id: String) async throws -> ProposalDetail {
        try await api.read { client in
            ProposalDetail(try await client.getProposal(path: .init(id: id)).ok.body.json)
        }
    }

    public func vote(id: String, choice: BallotChoice, submission: IdempotentSubmission) async throws {
        let body = Components.Schemas.CastVoteRequest(
            choice: choice == .yes ? .yes : .no)
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
            try await client.getMyPendingVotes().ok.body.json.map {
                PendingVote(id: $0.proposalId, cabalID: $0.cabalId)
            }
        }
    }

    public func lookups(for proposals: [Proposal]) async throws -> ProposalLookups {
        let cabalIDs = Set(proposals.map(\.cabalID))
        let symbols = Set(proposals.map(\.symbol))
        return try await withThrowingTaskGroup(of: Lookup.self) { group in
            for id in cabalIDs {
                group.addTask { .cabal(try await cabal(id: id)) }
            }
            for symbol in symbols {
                group.addTask { .asset(symbol, try await asset(symbol: symbol)) }
            }
            var cabals: [String: ProposalCabal] = [:]
            var assets: [String: ProposalAsset] = [:]
            for try await lookup in group {
                switch lookup {
                case .cabal(let cabal): cabals[cabal.id] = cabal
                case .asset(let symbol, let asset): assets[symbol] = asset
                }
            }
            return ProposalLookups(cabals: cabals, assets: assets)
        }
    }

    public func cabal(id: String) async throws -> ProposalCabal {
        try await api.read { client in
            ProposalCabal(try await client.getCabal(path: .init(id: id)).ok.body.json)
        }
    }

    func asset(symbol: String) async throws -> ProposalAsset {
        do {
            return try await api.read { client in
                ProposalAsset(try await client.getAsset(path: .init(symbol: symbol)).ok.body.json)
            }
        } catch APIError.problem(let problem) where problem.code.wire == "asset_not_found" {
            return ProposalAsset(unlisted: symbol)
        }
    }

    private enum Lookup: Sendable {
        case cabal(ProposalCabal)
        case asset(String, ProposalAsset)
    }
}
