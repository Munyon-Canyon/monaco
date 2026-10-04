import MonacoAPI

public struct LiveProposeService: ProposeService {
    private let api: APIClient

    public init(api: APIClient) {
        self.api = api
    }

    public func preview(groupID _: String, draft _: ProposalDraft) async throws {
        throw APIError.decoding("proposal preview is not implemented until #1928")
    }

    public func propose(
        groupID _: String, draft _: ProposalDraft, submission _: IdempotentSubmission
    ) async throws -> String {
        throw APIError.decoding("proposal submission is not implemented until #1929")
    }

    public func withdraw(proposalID: String, submission: IdempotentSubmission) async throws {
        _ = try await api.submit(
            submission, payload: ProposalID(proposalID), operation: Operations.DeleteProposal.id
        ) { client, key in
            try await client.deleteProposal(path: .init(id: proposalID), headers: .init(idempotencyKey: key)).ok
        }
    }
}

private struct ProposalID: Encodable, Sendable {
    let value: String

    init(_ value: String) {
        self.value = value
    }
}
