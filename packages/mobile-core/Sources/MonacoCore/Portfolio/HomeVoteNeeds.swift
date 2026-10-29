import Observation

@Observable
@MainActor
public final class HomeVoteNeeds {
    public private(set) var byCabal: [String: Int] = [:]

    public init() {}

    public func update(rows: [HomePendingVote], votedThisSession: Set<String>) {
        var counts: [String: Int] = [:]
        for row in rows where !votedThisSession.contains(row.id) {
            counts[row.cabalID, default: 0] += 1
        }
        if counts != byCabal { byCabal = counts }
    }

    public func text(for cabalID: String) -> String? {
        guard let count = byCabal[cabalID], count > 0 else { return nil }
        return count == 1 ? "1 vote open" : "\(count) votes open"
    }
}
