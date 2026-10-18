import MonacoAPI

public struct LiveProposeService: ProposeService {
    private let api: APIClient

    public init(api: APIClient) {
        self.api = api
    }

    public func preview(
        cabalID: String, draft: ProposalDraft, network: Components.Schemas.Network?
    ) async throws -> ProposePreview {
        let reply = try await api.read { client in
            try await client.getCabalProposalPreview(
                path: .init(id: cabalID), query: draft.previewQuery(network: network)
            ).ok.body.json
        }
        return ProposePreview(reply)
    }

    public func propose(
        cabalID: String, draft: ProposalDraft, network: Components.Schemas.Network?, submission: IdempotentSubmission
    ) async throws -> String {
        let request = draft.request(network: network)
        let reply = try await api.submit(submission, payload: request, operation: Operations.PostCabalProposal.id) {
            client, key in
            try await client.postCabalProposal(
                path: .init(id: cabalID), headers: .init(idempotencyKey: key), body: .json(request)
            ).created.body.json
        }
        return reply.id
    }

    public func withdraw(proposalID: String, submission: IdempotentSubmission) async throws {
        _ = try await api.submit(
            submission, payload: ProposalID(proposalID), operation: Operations.DeleteProposal.id
        ) { client, key in
            try await client.deleteProposal(path: .init(id: proposalID), headers: .init(idempotencyKey: key)).ok
        }
    }
}

extension ProposalDraft {
    fileprivate func previewQuery(
        network: Components.Schemas.Network?
    ) -> Operations.GetCabalProposalPreview.Input.Query {
        switch self {
        case .buy(let symbol, let usdcMicros, _):
            .init(kind: .buy, symbol: symbol, usdcMicros: usdcMicros > 0 ? usdcMicros : nil, network: network)
        case .sell(let symbol, let tokenAmount, _): .init(kind: .sell, symbol: symbol, tokenAmount: tokenAmount)
        }
    }

    fileprivate func request(network: Components.Schemas.Network?) -> Components.Schemas.ProposeTradeRequest {
        switch self {
        case .buy(let symbol, let usdcMicros, let thesis):
            .init(kind: .buy, symbol: symbol, usdcMicros: usdcMicros, thesis: thesis.trimmedOrNil, network: network)
        case .sell(let symbol, let tokenAmount, let thesis):
            .init(kind: .sell, symbol: symbol, tokenAmount: tokenAmount, thesis: thesis.trimmedOrNil)
        }
    }
}

extension String {
    fileprivate var trimmedOrNil: String? {
        let trimmed = trimmingCharacters(in: .whitespacesAndNewlines)
        return trimmed.isEmpty ? nil : trimmed
    }
}

private struct ProposalID: Encodable, Sendable {
    let value: String

    init(_ value: String) {
        self.value = value
    }
}
