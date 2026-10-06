import MonacoAPI
import MonacoCore
import MonacoTestClock
import MonacoTestSupport
import Synchronization
import XCTest

final class ChatSeenReporterTests: XCTestCase {
    private let seenReply = StubTransport.Reply.json(.ok, #"{"last_seen_at":"2026-10-03T12:00:00Z"}"#)

    private func makeReporter(
        _ transport: StubTransport, clock: TestClock, keys: [String] = []
    ) -> ChatSeenReporter {
        let counter = Counter()
        return ChatSeenReporter(
            cabalID: ChatFixtures.cabalID,
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            clock: clock,
            makeKey: { "key-\(counter.next())" }
        )
    }

    private func waitForCooldown(_ clock: TestClock) async {
        let ready = await clock.state.until { $0.pending == 1 }
        XCTAssertTrue(ready)
    }

    func testAppearingPostsOnceToTheSeenPath() async {
        let transport = StubTransport(seenReply)
        let reporter = makeReporter(transport, clock: TestClock())

        await reporter.setScreenVisible(true)
        await transport.waitForRequests(1)

        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.path), ["/v1/cabals/\(ChatFixtures.cabalID)/chat/seen"])
        XCTAssertEqual(sent.map(\.method), [.post])
    }

    func testFiveArrivalsInsideTheWindowMakeOneLeadingAndOneTrailingCall() async {
        let transport = StubTransport(seenReply)
        let clock = TestClock()
        let reporter = makeReporter(transport, clock: clock)
        await reporter.setScreenVisible(true)
        await transport.waitForRequests(1)
        await waitForCooldown(clock)
        clock.advance(by: ChatSeenReporter.window)
        while await reporter.isThrottled { await Task.yield() }
        let before = await transport.sent.count

        for _ in 0..<5 { await reporter.messageArrived() }
        await transport.waitForRequests(before + 1)
        let afterLeading = await transport.sent.count
        await waitForCooldown(clock)
        clock.advance(by: ChatSeenReporter.window)
        await transport.waitForRequests(before + 2)

        XCTAssertEqual(afterLeading - before, 1)
        let total = await transport.sent.count
        XCTAssertEqual(total - before, 2)
    }

    func testNothingIsSentWhileTheAppIsInTheBackground() async {
        let transport = StubTransport(seenReply)
        let clock = TestClock()
        let reporter = makeReporter(transport, clock: clock)
        await reporter.setScreenVisible(true)
        await waitForCooldown(clock)
        await transport.waitForRequests(1)
        await reporter.setAppActive(false)
        let before = await transport.sent.count

        await reporter.messageArrived()
        await reporter.messageArrived()
        clock.advance(by: ChatSeenReporter.window * 2)
        await reporter.setScreenVisible(false)

        let after = await transport.sent.count
        XCTAssertEqual(after, before)
    }

    func testAnArrivalBeforeGoingToTheBackgroundDoesNotFireItsTrailingCallThere() async {
        let transport = StubTransport(seenReply)
        let clock = TestClock()
        let reporter = makeReporter(transport, clock: clock)
        await reporter.setScreenVisible(true)
        await waitForCooldown(clock)
        await transport.waitForRequests(1)
        await reporter.messageArrived()
        await reporter.setAppActive(false)

        clock.advance(by: ChatSeenReporter.window)
        let quiet = await clock.state.until { $0.pending == 0 }
        XCTAssertTrue(quiet)

        let sent = await transport.sent.count
        XCTAssertEqual(sent, 1)
    }

    func testReturningToTheForegroundWhileVisiblePostsOnce() async {
        let transport = StubTransport(seenReply)
        let clock = TestClock()
        let reporter = makeReporter(transport, clock: clock)
        await reporter.setScreenVisible(true)
        await transport.waitForRequests(1)
        await reporter.setAppActive(false)
        let before = await transport.sent.count

        await reporter.setAppActive(true)
        await transport.waitForRequests(before + 1)

        let after = await transport.sent.count
        XCTAssertEqual(after - before, 1)
    }

    func testAnActiveAppWithAHiddenScreenPostsNothing() async {
        let transport = StubTransport(seenReply)
        let reporter = makeReporter(transport, clock: TestClock())

        await reporter.setAppActive(false)
        await reporter.setAppActive(true)
        await reporter.messageArrived()

        let sent = await transport.sent
        XCTAssertTrue(sent.isEmpty)
    }

    func testEveryCallCarriesAFreshIdempotencyKey() async {
        let transport = StubTransport(seenReply)
        let clock = TestClock()
        let reporter = makeReporter(transport, clock: clock)
        await reporter.setScreenVisible(true)
        await reporter.setAppActive(false)
        await reporter.setAppActive(true)
        await transport.waitForRequests(2)

        let keys = await ChatFixtures.idempotencyKeys(transport)
        XCTAssertEqual(keys.count, 2)
        XCTAssertEqual(Set(keys), ["key-1", "key-2"])
    }
}

private final class Counter: Sendable {
    private let value = Mutex(0)

    func next() -> Int {
        value.withLock {
            $0 += 1
            return $0
        }
    }
}
