import MonacoAPI
import Synchronization
import XCTest

@testable import MonacoCore

final class HintRefreshPolicyTests: XCTestCase {
    func testTenHintsDuringOneRefreshGiveOneTrailingRefresh() {
        var policy = HintRefreshPolicy()
        XCTAssertNil(policy.send(.refreshStarted))
        for _ in 0..<10 {
            XCTAssertNil(policy.send(.hint))
        }
        XCTAssertEqual(policy.send(.refreshFinished), .refreshNow)
        XCTAssertNil(policy.send(.refreshFinished))
    }

    func testThreeHintsWhileHiddenRefreshOnceOnBecomingVisible() {
        var policy = HintRefreshPolicy()
        XCTAssertNil(policy.send(.becameHidden))
        for _ in 0..<3 {
            XCTAssertNil(policy.send(.hint))
        }
        XCTAssertEqual(policy.send(.becameVisible), .refreshNow)
        XCTAssertNil(policy.send(.becameVisible))
    }

    func testNoHintWhileHiddenGivesNoRefresh() {
        var policy = HintRefreshPolicy()
        XCTAssertNil(policy.send(.becameHidden))
        XCTAssertNil(policy.send(.becameVisible))
    }

    func testResyncBehavesAsAHint() {
        var policy = HintRefreshPolicy()
        XCTAssertNil(policy.send(.refreshStarted))
        XCTAssertNil(policy.send(.resync))
        XCTAssertEqual(policy.send(.refreshFinished), .refreshNow)

        var hidden = HintRefreshPolicy()
        XCTAssertNil(hidden.send(.becameHidden))
        XCTAssertNil(hidden.send(.resync))
        XCTAssertEqual(hidden.send(.becameVisible), .refreshNow)
    }
}

@MainActor
final class HintRefresherTests: XCTestCase {
    func testFiveHintsDuringASlowRefreshCallRefreshTwice() async {
        let gate = RefreshGate()
        let refresher = HintRefresher { await gate.enter() }
        let (stream, continuation) = AsyncStream<Hint>.makeStream()
        let observing = Task { await refresher.observe(stream) }

        continuation.yield(Self.hint(1))
        await waitUntil(gate, count: 1)
        for index in 2...5 {
            continuation.yield(Self.hint(index))
            await Task.yield()
        }

        gate.releaseOne()
        await waitUntil(gate, count: 2)
        gate.releaseOne()
        for _ in 0..<40 {
            await Task.yield()
        }

        XCTAssertEqual(gate.count, 2)
        continuation.finish()
        await observing.value
    }

    func testHintsWhileHiddenRefreshOnceWhenShown() async {
        let gate = RefreshGate()
        let refresher = HintRefresher { await gate.enter() }
        let (stream, continuation) = AsyncStream<Hint>.makeStream()
        refresher.setVisible(false)
        let observing = Task { await refresher.observe(stream) }

        for index in 1...3 {
            continuation.yield(Self.hint(index))
        }
        continuation.finish()
        await observing.value

        XCTAssertEqual(gate.count, 0)
        refresher.setVisible(true)
        await waitUntil(gate, count: 1)
        gate.releaseOne()
        for _ in 0..<40 {
            await Task.yield()
        }
        XCTAssertEqual(gate.count, 1)
    }

    private static func hint(_ id: Int) -> Hint {
        Hint(key: "global", what: "feed", id: "\(id)") ?? .resync
    }

    private func waitUntil(_ gate: RefreshGate, count expected: Int) async {
        for _ in 0..<1_000 where gate.count < expected {
            await Task.yield()
        }
    }
}

private final class RefreshGate: Sendable {
    private let entered = Mutex(0)
    private let releaseWaiters = Mutex<[CheckedContinuation<Void, Never>]>([])

    func enter() async {
        entered.withLock { $0 += 1 }
        await withCheckedContinuation { continuation in
            releaseWaiters.withLock { $0.append(continuation) }
        }
    }

    var count: Int {
        entered.withLock { $0 }
    }

    func releaseOne() {
        let next = releaseWaiters.withLock { holders -> CheckedContinuation<Void, Never>? in
            guard !holders.isEmpty else { return nil }
            return holders.removeFirst()
        }
        next?.resume()
    }
}
