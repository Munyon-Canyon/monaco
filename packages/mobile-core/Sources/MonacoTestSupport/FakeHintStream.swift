import Foundation
import MonacoAPI

public actor FakeHintStream: HintSource {
    private var subscribers: [UUID: (filter: HintFilter, continuation: AsyncStream<Hint>.Continuation)] = [:]

    private var waiters: [(count: Int, continuation: CheckedContinuation<Void, Never>)] = []

    public init() {}

    public func waitForSubscribers(_ count: Int) async {
        if subscribers.count >= count { return }
        await withCheckedContinuation { waiters.append((count, $0)) }
    }

    public var subscriberCount: Int { subscribers.count }

    public nonisolated func hints(matching filter: HintFilter) -> AsyncStream<Hint> {
        let (stream, continuation) = AsyncStream.makeStream(of: Hint.self)
        let id = UUID()
        continuation.onTermination = { _ in Task { await self.remove(id) } }
        Task { await self.add(id, filter, continuation) }
        return stream
    }

    public func send(_ hint: Hint) {
        for entry in subscribers.values where entry.filter.matches(hint) {
            entry.continuation.yield(hint)
        }
    }

    private func add(_ id: UUID, _ filter: HintFilter, _ continuation: AsyncStream<Hint>.Continuation) {
        subscribers[id] = (filter, continuation)
        let ready = waiters.filter { subscribers.count >= $0.count }
        waiters.removeAll { subscribers.count >= $0.count }
        for waiter in ready { waiter.continuation.resume() }
    }

    private func remove(_ id: UUID) {
        subscribers[id] = nil
    }
}
