import Observation

@Observable
@MainActor
public final class HomeVoteNeeds {
    public private(set) var byCabal: [String: Int] = [:]

    public init() {}

    public func update(_ model: PendingVotesModel, votedThisSession: Set<String>) {
        var counts: [String: Int] = [:]
        for vote in model.needsVote where !votedThisSession.contains(vote.id) {
            guard let summary = model.details[vote.id]?.summary, summary.myBallot == nil else { continue }
            counts[summary.cabalID, default: 0] += 1
        }
        if counts != byCabal { byCabal = counts }
    }

    public func text(for cabalID: String) -> String? {
        guard let count = byCabal[cabalID], count > 0 else { return nil }
        return count == 1 ? "1 vote needed" : "\(count) votes needed"
    }
}
