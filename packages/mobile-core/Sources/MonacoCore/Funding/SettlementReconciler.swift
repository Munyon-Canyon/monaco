import Foundation
import MonacoAPI

public struct ReconcileSchedule: Sendable {
    public let interval: Duration
    public let retryFloor: Duration
    let clock: any Clock<Duration>

    public init(interval: Duration, retryFloor: Duration, clock: any Clock<Duration>) {
        self.interval = interval
        self.retryFloor = retryFloor
        self.clock = clock
    }

    public func delay(afterFailures failures: Int) -> Duration {
        guard failures > 0 else { return interval }
        return min(retryFloor * (1 << min(failures - 1, 8)), interval)
    }
}

enum SettlementReconciler {
    @MainActor
    static func run(
        hints: [AsyncStream<Hint>], schedule: ReconcileSchedule, read: () async -> Bool, isDone: () -> Bool
    ) async {
        let (signals, signal) = AsyncStream.makeStream(of: Void.self, bufferingPolicy: .bufferingNewest(1))
        let pumps = hints.map { stream in
            Task {
                for await _ in stream { signal.yield() }
            }
        }
        var wake: Task<Void, Never>?
        defer {
            for pump in pumps { pump.cancel() }
            wake?.cancel()
        }
        var ticks = signals.makeAsyncIterator()
        var failures = await read() ? 0 : 1
        while !isDone(), !Task.isCancelled {
            let delay = schedule.delay(afterFailures: failures)
            let clock = schedule.clock
            wake = Task {
                try? await clock.sleep(for: delay)
                if !Task.isCancelled { signal.yield() }
            }
            let signalled: Void? = await ticks.next()
            wake?.cancel()
            guard signalled != nil else { return }
            failures = await read() ? 0 : failures + 1
        }
    }
}
