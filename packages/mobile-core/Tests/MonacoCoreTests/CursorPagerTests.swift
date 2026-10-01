import Synchronization
import XCTest

@testable import MonacoCore

@MainActor
final class CursorPagerTests: XCTestCase {
    func testTwoPagesAppendInOrder() async {
        let pager = CursorPager<PageRow> { cursor in
            if cursor == nil {
                return ([PageRow(id: "a"), PageRow(id: "b")], "2")
            }
            return ([PageRow(id: "c")], nil)
        }

        await pager.loadFirst()
        await pager.loadMore()

        XCTAssertEqual(pager.items.map(\.id), ["a", "b", "c"])
        XCTAssertEqual(pager.phase, .exhausted)
    }

    func testADuplicateIdAcrossPagesAppearsOnce() async {
        let pager = CursorPager<PageRow> { cursor in
            if cursor == nil {
                return ([PageRow(id: "a", label: "first")], "2")
            }
            return ([PageRow(id: "a", label: "second"), PageRow(id: "b")], nil)
        }

        await pager.loadFirst()
        await pager.loadMore()

        XCTAssertEqual(pager.items, [PageRow(id: "a", label: "first"), PageRow(id: "b")])
    }

    func testLoadMoreTwiceConcurrentlyFetchesOnce() async {
        let probe = FetchProbe()
        let pager = CursorPager<PageRow> { cursor in
            if cursor == nil {
                return ([PageRow(id: "a")], "2")
            }
            return await probe.more()
        }
        await pager.loadFirst()

        async let first: Void = pager.loadMore()
        async let second: Void = pager.loadMore()
        var spins = 0
        while probe.callCount < 1, spins < 1_000 {
            await Task.yield()
            spins += 1
        }
        for _ in 0..<30 {
            await Task.yield()
        }

        XCTAssertEqual(probe.callCount, 1)
        probe.release()
        await first
        await second
        XCTAssertEqual(probe.callCount, 1)
        XCTAssertEqual(pager.items.map(\.id), ["a", "b"])
    }

    func testNilCursorStopsPaging() async {
        let counter = Counter()
        let pager = CursorPager<PageRow> { _ in
            counter.record()
            return ([PageRow(id: "a")], nil)
        }

        await pager.loadFirst()
        await pager.loadMore()

        XCTAssertEqual(counter.current, 1)
        XCTAssertEqual(pager.phase, .exhausted)
        XCTAssertEqual(pager.items.map(\.id), ["a"])
    }

    func testRefreshFirstPageKeepsPageTwo() async {
        let counter = Counter()
        let pager = CursorPager<PageRow> { cursor in
            let call = counter.bump()
            if cursor == "2" {
                return ([PageRow(id: "b"), PageRow(id: "c")], nil)
            }
            let label = call == 1 ? "old" : "new"
            return ([PageRow(id: "a", label: label)], "2")
        }

        await pager.loadFirst()
        await pager.loadMore()
        await pager.refreshFirstPage()

        XCTAssertEqual(
            pager.items,
            [PageRow(id: "a", label: "new"), PageRow(id: "b"), PageRow(id: "c")]
        )
        XCTAssertEqual(pager.phase, .exhausted)
    }

    func testRefreshDuringLoadMoreKeepsBoth() async {
        let firstPageCalls = Counter()
        let probe = FetchProbe()
        let pager = CursorPager<PageRow> { cursor in
            if cursor == nil {
                let call = firstPageCalls.bump()
                let label = call == 1 ? "old" : "new"
                return ([PageRow(id: "a", label: label)], "2")
            }
            return await probe.more()
        }

        await pager.loadFirst()

        async let more: Void = pager.loadMore()
        var spins = 0
        while probe.callCount < 1, spins < 1_000 {
            await Task.yield()
            spins += 1
        }

        await pager.refreshFirstPage()

        async let second: Void = pager.loadMore()
        for _ in 0..<30 {
            await Task.yield()
        }

        probe.release()
        await more
        await second

        XCTAssertEqual(pager.items, [PageRow(id: "a", label: "new"), PageRow(id: "b")])
        XCTAssertEqual(pager.phase, .exhausted)
        XCTAssertEqual(probe.callCount, 1)
    }

    func testLoadMoreDuringRefreshKeepsTheRefresh() async {
        let probe = GatedFirstPageProbe()
        let pager = CursorPager<PageRow> { cursor in
            if cursor == nil {
                return await probe.fetch()
            }
            return ([PageRow(id: "b")], nil)
        }

        await pager.loadFirst()

        async let refresh: Void = pager.refreshFirstPage()
        var spins = 0
        while probe.callCount < 2, spins < 1_000 {
            await Task.yield()
            spins += 1
        }

        await pager.loadMore()

        probe.release()
        await refresh

        XCTAssertEqual(pager.items, [PageRow(id: "a", label: "new"), PageRow(id: "b")])
        XCTAssertEqual(pager.phase, .exhausted)
    }

    func testRefreshDuringLoadFirstLeavesTheReloadInCharge() async {
        let holdReload = Mutex(false)
        let gate = SingleGate()
        let page2Calls = Counter()
        let page3Calls = Counter()

        let pager = CursorPager<PageRow> { cursor in
            if cursor == "2" {
                page2Calls.record()
                return ([PageRow(id: "c")], nil)
            }
            if cursor == "9" {
                page3Calls.record()
                return ([PageRow(id: "w")], nil)
            }
            if holdReload.withLock({ $0 }) {
                await gate.wait()
                return ([PageRow(id: "z")], "9")
            }
            return ([PageRow(id: "a"), PageRow(id: "b")], "2")
        }

        await pager.loadFirst()
        XCTAssertEqual(pager.items.map(\.id), ["a", "b"])
        XCTAssertEqual(pager.phase, .idle)

        holdReload.withLock { $0 = true }
        async let reload: Void = pager.loadFirst()

        var spins = 0
        while !gate.hasEntered, spins < 1_000 {
            await Task.yield()
            spins += 1
        }
        holdReload.withLock { $0 = false }

        await pager.refreshFirstPage()
        XCTAssertEqual(pager.phase, .loadingFirst)

        await pager.loadMore()
        XCTAssertEqual(page2Calls.current, 0)
        XCTAssertEqual(pager.phase, .loadingFirst)

        gate.release()
        await reload

        XCTAssertEqual(pager.items.map(\.id), ["z"])
        XCTAssertEqual(pager.phase, .idle)

        await pager.loadMore()
        XCTAssertEqual(page3Calls.current, 1)
        XCTAssertEqual(pager.items.map(\.id), ["z", "w"])
        XCTAssertEqual(pager.phase, .exhausted)
    }

    func testLoadFirstFailureSetsFailedPhase() async {
        let pager = CursorPager<PageRow> { _ in
            throw PagerTestError.boom
        }

        await pager.loadFirst()

        XCTAssertEqual(pager.items, [])
        guard case .failed = pager.phase else {
            return XCTFail("expected .failed, got \(pager.phase)")
        }
    }

    func testLoadMoreFailureSetsFailedPhaseAndResetsSingleFlight() async {
        let pager = CursorPager<PageRow> { cursor in
            if cursor == nil {
                return ([PageRow(id: "a")], "2")
            }
            throw PagerTestError.boom
        }

        await pager.loadFirst()
        await pager.loadMore()

        guard case .failed = pager.phase else {
            return XCTFail("expected .failed, got \(pager.phase)")
        }
        XCTAssertEqual(pager.items.map(\.id), ["a"])

        let retryCalls = Counter()
        let retryPager = CursorPager<PageRow> { cursor in
            if cursor == nil {
                return ([PageRow(id: "a")], "2")
            }
            retryCalls.record()
            throw PagerTestError.boom
        }
        await retryPager.loadFirst()
        await retryPager.loadMore()
        await retryPager.loadMore()
        XCTAssertEqual(retryCalls.current, 2)
    }

    func testRefreshFirstPageFailureSetsFailedPhase() async {
        let pager = CursorPager<PageRow> { _ in
            throw PagerTestError.boom
        }

        await pager.refreshFirstPage()

        guard case .failed = pager.phase else {
            return XCTFail("expected .failed, got \(pager.phase)")
        }
        XCTAssertEqual(pager.items, [])
    }
}

@MainActor
final class CursorPagerRefreshCursorTests: XCTestCase {
    func testRefreshFirstPageUpdatesCursorWhenTailIsEmpty() async {
        let firstPageCalls = Counter()
        let pager = CursorPager<PageRow> { cursor in
            if cursor == nil {
                return ([PageRow(id: "a")], firstPageCalls.bump() == 1 ? "2" : "3")
            }
            if cursor == "3" {
                return ([PageRow(id: "b")], nil)
            }
            return ([PageRow(id: "stale")], nil)
        }

        await pager.loadFirst()
        XCTAssertEqual(pager.phase, .idle)

        await pager.refreshFirstPage()

        XCTAssertEqual(pager.items.map(\.id), ["a"])
        XCTAssertEqual(pager.phase, .idle)

        await pager.loadMore()
        XCTAssertEqual(pager.items.map(\.id), ["a", "b"])
        XCTAssertEqual(pager.phase, .exhausted)
    }

    func testRefreshFirstPageSettlesExhaustedWhenTheListNowFitsOnePage() async {
        let firstPageCalls = Counter()
        let page2Calls = Counter()
        let pager = CursorPager<PageRow> { cursor in
            if cursor == nil {
                return ([PageRow(id: "a")], firstPageCalls.bump() == 1 ? "2" : nil)
            }
            page2Calls.record()
            return ([PageRow(id: "b")], nil)
        }

        await pager.loadFirst()
        XCTAssertEqual(pager.phase, .idle)

        await pager.refreshFirstPage()

        XCTAssertEqual(pager.items.map(\.id), ["a"])
        XCTAssertEqual(pager.phase, .exhausted)

        await pager.loadMore()
        XCTAssertEqual(page2Calls.current, 0)
        XCTAssertEqual(pager.items.map(\.id), ["a"])
    }
}

private enum PagerTestError: Error {
    case boom
}

private struct PageRow: Identifiable, Equatable, Sendable {
    let id: String
    var label: String = ""
}

private final class Counter: Sendable {
    private let value = Mutex(0)

    func bump() -> Int {
        value.withLock { current in
            current += 1
            return current
        }
    }

    func record() {
        _ = bump()
    }

    var current: Int {
        value.withLock { $0 }
    }
}

private final class SingleGate: Sendable {
    private struct State {
        var entered = false
        var released = false
        var waiters: [CheckedContinuation<Void, Never>] = []
    }
    private let state = Mutex(State())

    func wait() async {
        let resumeNow = state.withLock { s -> Bool in
            s.entered = true
            return s.released
        }
        if resumeNow { return }
        await withCheckedContinuation { continuation in
            let resume = state.withLock { s -> Bool in
                if s.released { return true }
                s.waiters.append(continuation)
                return false
            }
            if resume {
                continuation.resume()
            }
        }
    }

    var hasEntered: Bool {
        state.withLock { $0.entered }
    }

    func release() {
        let pending = state.withLock { s -> [CheckedContinuation<Void, Never>] in
            s.released = true
            let copy = s.waiters
            s.waiters.removeAll()
            return copy
        }
        for waiter in pending {
            waiter.resume()
        }
    }
}

private final class FetchProbe: Sendable {
    private struct State {
        var calls = 0
        var released = false
        var waiters: [CheckedContinuation<Void, Never>] = []
    }
    private let state = Mutex(State())

    func more() async -> (items: [PageRow], nextCursor: String?) {
        let resumeNow = state.withLock { s -> Bool in
            s.calls += 1
            return s.released
        }
        if !resumeNow {
            await withCheckedContinuation { continuation in
                let resume = state.withLock { s -> Bool in
                    if s.released { return true }
                    s.waiters.append(continuation)
                    return false
                }
                if resume {
                    continuation.resume()
                }
            }
        }
        return ([PageRow(id: "b")], nil)
    }

    var callCount: Int {
        state.withLock { $0.calls }
    }

    func release() {
        let pending = state.withLock { s -> [CheckedContinuation<Void, Never>] in
            s.released = true
            let copy = s.waiters
            s.waiters.removeAll()
            return copy
        }
        for waiter in pending {
            waiter.resume()
        }
    }
}

private final class GatedFirstPageProbe: Sendable {
    private struct State {
        var calls = 0
        var released = false
        var waiters: [CheckedContinuation<Void, Never>] = []
    }
    private let state = Mutex(State())

    func fetch() async -> (items: [PageRow], nextCursor: String?) {
        let call = state.withLock { s -> Int in
            s.calls += 1
            return s.calls
        }
        if call == 1 {
            return ([PageRow(id: "a", label: "old")], "2")
        }
        let resumeNow = state.withLock { $0.released }
        if !resumeNow {
            await withCheckedContinuation { continuation in
                let resume = state.withLock { s -> Bool in
                    if s.released { return true }
                    s.waiters.append(continuation)
                    return false
                }
                if resume {
                    continuation.resume()
                }
            }
        }
        return ([PageRow(id: "a", label: "new")], "2")
    }

    var callCount: Int {
        state.withLock { $0.calls }
    }

    func release() {
        let pending = state.withLock { s -> [CheckedContinuation<Void, Never>] in
            s.released = true
            let copy = s.waiters
            s.waiters.removeAll()
            return copy
        }
        for waiter in pending {
            waiter.resume()
        }
    }
}
