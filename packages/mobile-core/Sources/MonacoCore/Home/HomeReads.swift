import MonacoAPI
import Observation

public enum HomeReadSlot: CaseIterable, Sendable {
    case portfolio
    case balance
    case board
}

public enum HomeReadStatus: Equatable, Sendable {
    case pending
    case settled
    case offline
    case failed

    public init<Value>(_ state: LoadState<Value>) {
        switch state {
        case .idle, .loading: self = .pending
        case .loaded: self = .settled
        case .failed(let error): self.init(failure: error)
        }
    }

    public init(_ phase: LeaderboardLoader.Phase) {
        switch phase {
        case .loading: self = .pending
        case .empty, .loaded: self = .settled
        case .failed(let error): self.init(failure: error)
        }
    }

    private init(failure error: APIError) {
        if case .transport = error {
            self = .offline
        } else {
            self = .failed
        }
    }
}

public struct HomeErrorPlan: Equatable, Sendable {
    public private(set) var statuses: [HomeReadSlot: HomeReadStatus] = [:]

    public init() {}

    public mutating func report(_ slot: HomeReadSlot, _ status: HomeReadStatus) {
        statuses[slot] = status
    }

    public var showsSharedRow: Bool {
        HomeReadSlot.allCases.allSatisfy { statuses[$0] == .offline }
    }

    public func showsOwnRow(_ slot: HomeReadSlot) -> Bool {
        guard let status = statuses[slot], status == .offline || status == .failed else { return false }
        return !showsSharedRow
    }
}

@Observable
@MainActor
public final class HomeReads {
    public private(set) var plan = HomeErrorPlan()

    public init() {}

    public func report(_ slot: HomeReadSlot, _ status: HomeReadStatus) {
        plan.report(slot, status)
    }
}
