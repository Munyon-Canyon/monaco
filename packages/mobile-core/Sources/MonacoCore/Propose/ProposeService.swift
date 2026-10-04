import MonacoAPI

public protocol ProposeService: Sendable {
    func preview(groupID: String, draft: ProposalDraft) async throws
    func propose(groupID: String, draft: ProposalDraft, submission: IdempotentSubmission) async throws -> String
    func withdraw(proposalID: String, submission: IdempotentSubmission) async throws
}
