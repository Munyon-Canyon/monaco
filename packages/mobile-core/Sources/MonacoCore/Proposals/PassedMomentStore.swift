import Foundation

@MainActor
public final class PassedMomentStore {
    public static let window: TimeInterval = 7 * 86_400
    public static let retention: TimeInterval = 2 * window

    private let defaults: UserDefaults
    private let key: String
    private let now: () -> Date

    public init(defaults: UserDefaults, accountID: String, now: @escaping () -> Date) {
        self.defaults = defaults
        self.key = "passedMomentsSeen.\(accountID)"
        self.now = now
    }

    public func shouldShow(detail: ProposalDetail) -> Bool {
        let summary = detail.summary
        guard summary.status == .passed || summary.status == .executed, summary.myBallot != nil,
            ProposalResultCopy.isFilled(summary.swap), let filledAt = summary.swap?.filledAt
        else { return false }
        guard filledAt >= now().addingTimeInterval(-Self.window) else { return false }
        return seen[summary.id] == nil
    }

    public func markSeen(id: String, filledAt: Date) {
        var entries = seen
        entries[id] = filledAt.timeIntervalSince1970
        let cutoff = now().addingTimeInterval(-Self.retention).timeIntervalSince1970
        entries = entries.filter { $0.value >= cutoff }
        defaults.set(entries, forKey: key)
    }

    private var seen: [String: Double] {
        defaults.dictionary(forKey: key) as? [String: Double] ?? [:]
    }
}

@MainActor
public final class PassedMomentBook {
    private let defaults: UserDefaults
    private let now: () -> Date
    private var stores: [String: PassedMomentStore] = [:]

    public init(defaults: UserDefaults, now: @escaping () -> Date = Date.init) {
        self.defaults = defaults
        self.now = now
    }

    public func store(for accountID: String) -> PassedMomentStore {
        if let existing = stores[accountID] { return existing }
        let created = PassedMomentStore(defaults: defaults, accountID: accountID, now: now)
        stores[accountID] = created
        return created
    }
}
