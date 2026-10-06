public struct BoardRowKey: Hashable, Sendable {
    public let id: String
    public let rank: Int

    public init(id: String, rank: Int) {
        self.id = id
        self.rank = rank
    }
}

public func rankChanges(old: [BoardRowKey], new: [BoardRowKey]) -> [String: Int] {
    let before = Dictionary(old.map { ($0.id, $0.rank) }, uniquingKeysWith: { first, _ in first })
    var changes: [String: Int] = [:]
    for row in new {
        guard let was = before[row.id], was != row.rank else { continue }
        changes[row.id] = was - row.rank
    }
    return changes
}
