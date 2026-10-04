import MonacoAPI
import Observation

public struct ProposalToast: Equatable, Sendable {
    public let serial: Int
    public let message: String
    public let isSuccess: Bool
}

@Observable
@MainActor
public final class ProposalVoting {
    public private(set) var casting: Set<String> = []
    public private(set) var toast: ProposalToast?
    private var submissions: [String: IdempotentSubmission] = [:]
    private var serial = 0
    private let repository: ProposalsRepository

    init(repository: ProposalsRepository) {
        self.repository = repository
    }

    func cast(_ choice: BallotChoice, on proposalID: String) async -> Bool {
        guard !casting.contains(proposalID) else { return false }
        casting.insert(proposalID)
        defer { casting.remove(proposalID) }
        let submission = submissions[proposalID] ?? IdempotentSubmission()
        submissions[proposalID] = submission
        do {
            try await repository.vote(id: proposalID, choice: choice, submission: submission)
            show(ProposalCardCopy.voteToast, success: true)
            return true
        } catch {
            show(ToastCopy.message(for: APIError(error)), success: false)
            return false
        }
    }

    func show(_ message: String, success: Bool) {
        serial += 1
        toast = ProposalToast(serial: serial, message: message, isSuccess: success)
    }
}

@MainActor
final class ProposalReloadHook {
    var run: (@MainActor () async -> Void)?
}
