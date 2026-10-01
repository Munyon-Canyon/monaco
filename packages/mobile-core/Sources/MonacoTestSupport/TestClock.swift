import Foundation

/// A clock that moves only when a test calls `advance(by:)`.
/// Sleepers whose deadline has passed resume in deadline order, and sleepers
/// that share a deadline resume in the order they called `sleep`.
public final class TestClock: Clock, @unchecked Sendable {
    public struct Instant: InstantProtocol {
        public var offset: Duration

        public init(offset: Duration) {
            self.offset = offset
        }

        public func advanced(by duration: Duration) -> Instant {
            Instant(offset: offset + duration)
        }

        public func duration(to other: Instant) -> Duration {
            other.offset - offset
        }

        public static func < (lhs: Instant, rhs: Instant) -> Bool {
            lhs.offset < rhs.offset
        }
    }

    public struct State: Sendable {
        public var now = Instant(offset: .zero)
        /// Every sleep asked for, in order, as a duration from the time it was asked.
        public var requested: [Duration] = []
        /// Deadlines resumed by `advance(by:)`, in the order they were resumed.
        public var resumed: [Duration] = []
        /// Sleeper ids resumed by `advance(by:)`, in that same order.
        var resumeOrder: [Int] = []
        fileprivate var sleepers: [Int: Sleeper] = [:]
        fileprivate var cancelled: Set<Int> = []
        fileprivate var nextID = 0

        public var pending: Int { sleepers.count }
    }

    fileprivate struct Sleeper: Sendable {
        let deadline: Instant
        let continuation: CheckedContinuation<Void, any Error>
    }

    public let state: Watched<State>
    public var now: Instant { state.current.now }
    public var minimumResolution: Duration { .zero }

    public init() {
        state = Watched(State())
    }

    /// Ids in the order `advance(by:)` resumes them: earlier deadlines first, and
    /// equal deadlines in the order `sleep` registered them (`id` is that order).
    static func orderedIDs(_ entries: [(id: Int, deadline: Instant)]) -> [Int] {
        entries.sorted { lhs, rhs in
            if lhs.deadline == rhs.deadline { return lhs.id < rhs.id }
            return lhs.deadline < rhs.deadline
        }.map(\.id)
    }

    /// Moves the clock forward and resumes every sleeper whose deadline has passed, earliest first.
    public func advance(by duration: Duration) {
        let due = state.mutate { state in
            state.now = state.now.advanced(by: duration)
            let ids = Self.orderedIDs(
                state.sleepers.compactMap { id, sleeper in
                    sleeper.deadline <= state.now ? (id: id, deadline: sleeper.deadline) : nil
                })
            var continuations: [CheckedContinuation<Void, any Error>] = []
            continuations.reserveCapacity(ids.count)
            for id in ids {
                guard let sleeper = state.sleepers.removeValue(forKey: id) else { continue }
                state.resumed.append(sleeper.deadline.offset)
                state.resumeOrder.append(id)
                continuations.append(sleeper.continuation)
            }
            return continuations
        }
        for continuation in due {
            continuation.resume()
        }
    }

    public func sleep(until deadline: Instant, tolerance _: Duration? = nil) async throws {
        let id = state.mutate { state in
            state.nextID += 1
            return state.nextID
        }
        try await withTaskCancellationHandler {
            try await withCheckedThrowingContinuation { (continuation: CheckedContinuation<Void, any Error>) in
                let outcome: Result<Void, any Error>? = state.mutate { state in
                    if state.cancelled.remove(id) != nil { return .failure(CancellationError()) }
                    state.requested.append(state.now.duration(to: deadline))
                    if deadline <= state.now { return .success(()) }
                    state.sleepers[id] = Sleeper(deadline: deadline, continuation: continuation)
                    return nil
                }
                if let outcome { continuation.resume(with: outcome) }
            }
        } onCancel: {
            let sleeper = state.mutate { state in
                let sleeper = state.sleepers.removeValue(forKey: id)
                if sleeper == nil { state.cancelled.insert(id) }
                return sleeper
            }
            sleeper?.continuation.resume(throwing: CancellationError())
        }
    }
}
