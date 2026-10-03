import Foundation
import MonacoTestSupport
import XCTest

@testable import MonacoAPI

#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

final class TimeoutMiddlewareTests: XCTestCase {
    private func client(_ transport: StubTransport, clock: SkippingClock) -> APIClient {
        APIClient(
            serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport, clock: clock)
    }

    func testAReadTimesOutAt15Seconds() async throws {
        let clock = SkippingClock()
        let started = ContinuousClock.now

        await assertThrows(.transport(URLError(.timedOut))) {
            try await client(StubTransport(.hang), clock: clock).healthz()
        }

        XCTAssertEqual(clock.slept, [.seconds(15)])
        XCTAssertLessThan(ContinuousClock.now - started, .milliseconds(100))
    }

    func testAKeyedWriteTimesOutAt60Seconds() async throws {
        let clock = SkippingClock()
        let submission = IdempotentSubmission { "key-1" }
        let started = ContinuousClock.now

        await assertThrows(.transport(URLError(.timedOut))) {
            _ = try await client(StubTransport(.hang), clock: clock).ping(submission)
        }

        XCTAssertEqual(clock.slept, [.seconds(60)])
        XCTAssertLessThan(ContinuousClock.now - started, .milliseconds(100))
        XCTAssertTrue(submission.hasPendingKey, "a timed-out write keeps its key so the retry replays")
    }

    func testASessionOpenTimesOutAt15SecondsWithoutAnIdempotencyKey() {
        let request = HTTPRequest(
            method: .post, scheme: "https", authority: "example.com", path: "/v1/auth/session"
        )
        XCTAssertEqual(
            TimeoutMiddleware.budget(for: request, operationID: "postAuthSession"),
            .seconds(15)
        )
    }

    func testAnAnswerInsideTheBudgetIsReturned() async throws {
        try await client(StubTransport.ok("ok\n"), clock: SkippingClock(skips: false)).healthz()
    }
}

/// A clock whose `sleep` records the requested duration and, when `skips`, returns at once
/// as if that time had passed. With `skips` false it never wakes until cancelled.
final class SkippingClock: Clock, @unchecked Sendable {
    struct Instant: InstantProtocol {
        var offset: Duration

        func advanced(by duration: Duration) -> Instant { Instant(offset: offset + duration) }
        func duration(to other: Instant) -> Duration { other.offset - offset }
        static func < (lhs: Instant, rhs: Instant) -> Bool { lhs.offset < rhs.offset }
    }

    private let lock = NSLock()
    private let skips: Bool
    private var current = Instant(offset: .zero)
    private var recorded: [Duration] = []

    init(skips: Bool = true) {
        self.skips = skips
    }

    var now: Instant {
        lock.lock()
        defer { lock.unlock() }
        return current
    }

    var minimumResolution: Duration { .zero }

    var slept: [Duration] {
        lock.lock()
        defer { lock.unlock() }
        return recorded
    }

    func sleep(until deadline: Instant, tolerance _: Duration?) async throws {
        record(deadline)
        if !skips {
            try await ContinuousClock().sleep(for: .seconds(86_400))
        }
    }

    private func record(_ deadline: Instant) {
        lock.lock()
        defer { lock.unlock() }
        recorded.append(current.duration(to: deadline))
        if skips { current = max(current, deadline) }
    }
}
