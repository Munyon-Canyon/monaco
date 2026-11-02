import MonacoAPI
import MonacoGovernance
import Observation

@Observable
@MainActor
public final class ProposalVoteModel {
    public private(set) var ballots: [String: String] = [:]
    public private(set) var errorMessage: String?
    public private(set) var voteRefusal: VoteRefusal?
    public private(set) var undoRefusal: UndoRefusal?
    private var confirmedTallies: [String: ProposalTally] = [:]
    private var acknowledged: Set<String> = []
    private let repository: ProposalsRepository
    private var submissions: [String: IdempotentSubmission] = [:]
    private var retractions: [String: IdempotentSubmission] = [:]

    public var votedIDs: Set<String> { Set(ballots.keys).union(acknowledged) }

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

    public func acknowledge(_ summary: ProposalSummary) {
        guard summary.myBallot != nil else { return }
        ballots[summary.id] = nil
        confirmedTallies[summary.id] = nil
        acknowledged.insert(summary.id)
    }

    public func vote(_ choice: String, on summary: ProposalSummary) async -> Bool {
        await cast(choice, on: summary) != nil
    }

    public func cast(_ choice: String, on summary: ProposalSummary) async -> ProposalVoteResult? {
        let previous = ballots[summary.id]
        confirmedTallies[summary.id] = nil
        ballots[summary.id] = choice
        errorMessage = nil
        voteRefusal = nil
        let submission = submissions[summary.id] ?? IdempotentSubmission()
        submissions[summary.id] = submission
        do {
            let result = try await repository.vote(id: summary.id, choice: choice, submission: submission)
            if result.status == .open { confirmedTallies[summary.id] = result.tally }
            retractions[summary.id] = nil
            return result
        } catch {
            ballots[summary.id] = previous
            let failure = APIError(error)
            errorMessage = ToastCopy.message(for: failure)
            voteRefusal = VoteRefusal(failure)
            return nil
        }
    }

    public func undo(on summary: ProposalSummary) async -> ProposalRetractResult? {
        errorMessage = nil
        undoRefusal = nil
        let submission = retractions[summary.id] ?? IdempotentSubmission()
        retractions[summary.id] = submission
        do {
            let result = try await repository.retractVote(id: summary.id, submission: submission)
            clearOverlay(for: summary.id)
            acknowledged.remove(summary.id)
            submissions[summary.id] = nil
            return result
        } catch {
            let failure = APIError(error)
            errorMessage = ToastCopy.message(for: failure)
            undoRefusal = UndoRefusal(failure)
            return nil
        }
    }

    private func clearOverlay(for id: String) {
        ballots[id] = nil
        confirmedTallies[id] = nil
    }
}
