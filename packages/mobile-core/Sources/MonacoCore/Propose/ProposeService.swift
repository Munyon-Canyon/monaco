import MonacoAPI

public protocol ProposeService: Sendable {
    func preview(cabalID: String, draft: ProposalDraft) async throws -> ProposePreview
    func propose(cabalID: String, draft: ProposalDraft, submission: IdempotentSubmission) async throws -> String
    func withdraw(proposalID: String, submission: IdempotentSubmission) async throws
}
