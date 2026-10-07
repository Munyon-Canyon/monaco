import MonacoAPI
import Observation

@Observable
@MainActor
public final class ProposalVoteModel {
    public private(set) var ballots: [String: String] = [:]
    public private(set) var errorMessage: String?
    private var confirmedTallies: [String: ProposalTally] = [:]
    private let repository: ProposalsRepository
    private var submissions: [String: IdempotentSubmission] = [:]

    public var votedIDs: Set<String> { Set(ballots.keys) }

    public init(repository: ProposalsRepository) {
        self.repository = repository
    }

    public func applying(_ summary: ProposalSummary) -> ProposalSummary {
        guard let choice = ballots[summary.id], choice != summary.myBallot else { return summary }
        if let confirmed = confirmedTallies[summary.id] {
            return ProposalSummary(summary, ballot: choice, tally: confirmed)
        }
        var yes = summary.tally.yes
        var no = summary.tally.no
        if summary.myBallot == "yes" { yes -= 1 }
        if summary.myBallot == "no" { no -= 1 }
        if choice == "yes" { yes += 1 } else { no += 1 }
        let tally = ProposalTally(yes: yes, no: no, voters: summary.tally.voters, needed: summary.tally.needed)
        return ProposalSummary(summary, ballot: choice, tally: tally)
    }

    public func vote(_ choice: String, on summary: ProposalSummary) async -> Bool {
        let previous = ballots[summary.id]
        confirmedTallies[summary.id] = nil
        ballots[summary.id] = choice
        errorMessage = nil
        let submission = submissions[summary.id] ?? IdempotentSubmission()
        submissions[summary.id] = submission
        do {
            let result = try await repository.vote(id: summary.id, choice: choice, submission: submission)
            if result.status == .open { confirmedTallies[summary.id] = result.tally }
            return true
        } catch {
            ballots[summary.id] = previous
            errorMessage = ToastCopy.message(for: APIError(error))
            return false
        }
    }
}
