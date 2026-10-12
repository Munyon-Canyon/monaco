import MonacoAPI

public protocol ProposeService: Sendable {
    func preview(
        cabalID: String, draft: ProposalDraft, network: Components.Schemas.Network?
    ) async throws -> ProposePreview
    func potValue(cabalID: String) async throws -> Int64
    func propose(
        cabalID: String, draft: ProposalDraft, network: Components.Schemas.Network?, submission: IdempotentSubmission
    ) async throws -> String
    func withdraw(proposalID: String, submission: IdempotentSubmission) async throws
}
