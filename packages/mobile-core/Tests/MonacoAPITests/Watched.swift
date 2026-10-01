import Foundation
import MonacoTestClock

extension Watched {
    /// A live round trip (the hint integration test) still needs a real bound. Unit tests use
    /// `until(_:)`, which does not sleep.
    func until(within limit: Duration, _ predicate: @escaping @Sendable (Value) -> Bool) async -> Bool {
        let clock = ContinuousClock()
        let start = clock.now
        return await until(within: limit, sleep: { _ in
            while clock.now - start < limit {
                await Task.yield()
            }
        }, predicate)
    }
}
