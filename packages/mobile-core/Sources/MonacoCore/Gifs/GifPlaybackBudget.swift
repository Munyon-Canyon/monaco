import Observation

@Observable @MainActor
public final class GifPlaybackBudget {
    public static let sheet = GifPlaybackBudget(limit: 6)

    @ObservationIgnored private let limit: Int
    @ObservationIgnored private var active: Set<String> = []
    public private(set) var revision = 0

    public init(limit: Int) {
        self.limit = limit
    }

    public func claim(_ id: String) -> Bool {
        if active.contains(id) { return true }
        guard active.count < limit else { return false }
        active.insert(id)
        return true
    }

    public func release(_ id: String) {
        if active.remove(id) != nil { revision += 1 }
    }
}
