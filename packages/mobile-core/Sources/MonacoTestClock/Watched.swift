import Foundation

public final class Watched<Value: Sendable>: @unchecked Sendable {
    private let lock = NSLock()
    private var value: Value
    private var nextID = 0
    private var watchers:
        [Int: (predicate: @Sendable (Value) -> Bool, continuation: CheckedContinuation<Bool, Never>)] = [:]

    public init(_ value: Value) {
        self.value = value
    }

    public var current: Value {
        lock.lock()
        defer { lock.unlock() }
        return value
    }

    @discardableResult
    public func mutate<Result>(_ body: (inout Value) -> Result) -> Result {
        lock.lock()
        let result = body(&value)
        let ready = watchers.filter { $0.value.predicate(value) }
        for id in ready.keys { watchers[id] = nil }
        lock.unlock()
        for watcher in ready.values { watcher.continuation.resume(returning: true) }
        return result
    }

    public func until(_ predicate: @escaping @Sendable (Value) -> Bool) async -> Bool {
        await withCheckedContinuation { continuation in
            lock.lock()
            if predicate(value) {
                lock.unlock()
                continuation.resume(returning: true)
                return
            }
            let id = nextID
            nextID += 1
            watchers[id] = (predicate, continuation)
            lock.unlock()
        }
    }

    public func until(
        within limit: Duration,
        sleep: @escaping @Sendable (Duration) async -> Void,
        _ predicate: @escaping @Sendable (Value) -> Bool
    ) async -> Bool {
        await withCheckedContinuation { continuation in
            lock.lock()
            if predicate(value) {
                lock.unlock()
                continuation.resume(returning: true)
                return
            }
            let id = nextID
            nextID += 1
            watchers[id] = (predicate, continuation)
            lock.unlock()
            Task {
                await sleep(limit)
                self.expire(id)
            }
        }
    }

    private func expire(_ id: Int) {
        lock.lock()
        let watcher = watchers.removeValue(forKey: id)
        lock.unlock()
        watcher?.continuation.resume(returning: false)
    }
}
