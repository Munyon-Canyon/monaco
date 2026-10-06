import Foundation
import MonacoAPI
import XCTest

@testable import MonacoAPI

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
        let refresher = HintRefresher(
            refresh: { await gate.enter() },
            didObserveHint: { gate.observeHint() }
        )
        let (stream, continuation) = AsyncStream<Hint>.makeStream()
        let observing = Task { await refresher.observe(stream) }

        continuation.yield(Self.hint(1))
        await gate.waitForRefreshes(1)
        for index in 2...5 {
            continuation.yield(Self.hint(index))
        }
        await gate.waitForHints(5)

        gate.releaseOne()
        await gate.waitForRefreshes(2)
        gate.releaseOne()
        continuation.finish()
        await observing.value
        XCTAssertEqual(gate.refreshCount, 2)
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

        XCTAssertEqual(gate.refreshCount, 0)
        refresher.setVisible(true)
        await gate.waitForRefreshes(1)
        gate.releaseOne()
        await gate.waitForFinishes(1)
        XCTAssertEqual(gate.refreshCount, 1)
    }

    func testAResyncOnEveryStreamRefreshesOnce() async {
        let gate = RefreshGate()
        let refresher = HintRefresher(
            refresh: { await gate.enter() },
            didObserveHint: { gate.observeHint() }
        )
        let (first, firstContinuation) = AsyncStream<Hint>.makeStream()
        let (second, secondContinuation) = AsyncStream<Hint>.makeStream()
        let observing = Task { await refresher.observe([first, second]) }

        firstContinuation.yield(.resync)
        await gate.waitForRefreshes(1)
        gate.releaseOne()
        await gate.waitForFinishes(1)
        secondContinuation.yield(.resync)
        await gate.waitForHints(2)
        secondContinuation.yield(Self.hint(1))
        await gate.waitForRefreshes(2)
        gate.releaseOne()
        firstContinuation.finish()
        secondContinuation.finish()
        await observing.value

        XCTAssertEqual(gate.refreshCount, 2)
    }

    private static func hint(_ id: Int) -> Hint {
        Hint(key: "global", what: "feed", id: "\(id)") ?? .resync
    }

}

@MainActor
private final class RefreshGate {
    private let refreshes = Signal()
    private let hints = Signal()
    private let releases = Signal()
    private let finishes = Signal()

    func enter() async {
        await releases.waitUntil(refreshes.signal())
        finishes.signal()
    }

    func observeHint() {
        hints.signal()
    }

    var refreshCount: Int { refreshes.count }

    func waitForRefreshes(_ expected: Int) async {
        await refreshes.waitUntil(expected)
    }

    func waitForHints(_ expected: Int) async {
        await hints.waitUntil(expected)
    }

    func waitForFinishes(_ expected: Int) async {
        await finishes.waitUntil(expected)
    }

    func releaseOne() {
        releases.signal()
    }
}

@MainActor
private final class Signal {
    private let lock = NSLock()
    private var value = 0
    private var waiters: [(Int, CheckedContinuation<Void, Never>)] = []

    var count: Int { lock.withLock { value } }

    @discardableResult
    func signal() -> Int {
        let (count, ready) = lock.withLock { () -> (Int, [CheckedContinuation<Void, Never>]) in
            value += 1
            let ready = waiters.filter { $0.0 <= value }
            waiters.removeAll { $0.0 <= value }
            return (value, ready.map(\.1))
        }
        for continuation in ready {
            continuation.resume()
        }
        return count
    }

    func waitUntil(_ expected: Int) async {
        await withCheckedContinuation { continuation in
            let resume = lock.withLock { () -> Bool in
                guard value < expected else { return true }
                waiters.append((expected, continuation))
                return false
            }
            if resume { continuation.resume() }
        }
    }
}
