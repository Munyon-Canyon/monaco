import Foundation

/// A clock that moves only when a test calls `advance(by:)`.
final class TestClock: Clock, @unchecked Sendable {
    struct Instant: InstantProtocol {
        var offset: Duration

        func advanced(by duration: Duration) -> Instant { Instant(offset: offset + duration) }
        func duration(to other: Instant) -> Duration { other.offset - offset }
        static func < (lhs: Instant, rhs: Instant) -> Bool { lhs.offset < rhs.offset }
    }

    struct State: Sendable {
        var now = Instant(offset: .zero)
        /// Every sleep asked for, in order, as a duration from the time it was asked.
        var requested: [Duration] = []
        fileprivate var sleepers: [Int: Sleeper] = [:]
        fileprivate var cancelled: Set<Int> = []
        fileprivate var nextID = 0

        var pending: Int { sleepers.count }
    }

    fileprivate struct Sleeper: Sendable {
        let deadline: Instant
        let continuation: CheckedContinuation<Void, any Error>
    }

    let state = Watched(State())

    var now: Instant { state.current.now }
    var minimumResolution: Duration { .zero }

    func advance(by duration: Duration) {
        let due = state.mutate { state in
            state.now = state.now.advanced(by: duration)
            let due = state.sleepers.filter { $0.value.deadline <= state.now }.sorted { $0.value.deadline < $1.value.deadline }
            for (id, _) in due { state.sleepers[id] = nil }
            return due.map(\.value.continuation)
        }
        for continuation in due { continuation.resume() }
    }

    func sleep(until deadline: Instant, tolerance _: Duration? = nil) async throws {
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
