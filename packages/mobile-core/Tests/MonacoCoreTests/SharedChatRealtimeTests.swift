import Foundation
import MonacoAPI
import MonacoTestSupport
import Synchronization
import XCTest

@testable import MonacoCore

private final class Counts: Sendable {
    struct Values { var tokens = 0, closed = 0, detached: [String] = [] }
    let store = Mutex(Values())
    func withLock<R>(_ body: (inout Values) -> R) -> R { store.withLock { body(&$0) } }
}

private final class FakeLink: ChatRealtimeLink {
    private let counts: Counts

    init(tokens: Counts) {
        counts = tokens
        counts.withLock { $0.tokens += 1 }
    }

    func events(cabalId _: String) -> AsyncStream<ChatRealtimeEvent> {
        AsyncStream { $0.finish() }
    }

    func detach(cabalId: String) { counts.withLock { $0.detached.append(cabalId) } }
    func close() { counts.withLock { $0.closed += 1 } }
}

final class SharedChatRealtimeTests: XCTestCase {
    func testTwentyChatScreensInARowMakeOneConnectionAndOneTokenRequest() {
        let counts = Counts()
        let realtime = SharedChatRealtime { FakeLink(tokens: counts) }

        for index in 0..<20 {
            let cabal = "cabal-\(index % 3)"
            _ = realtime.events(cabalId: cabal)
            realtime.detach(cabalId: cabal)
        }

        let seen = counts.withLock { $0 }
        XCTAssertEqual(seen.tokens, 1)
        XCTAssertEqual(seen.closed, 0)
        XCTAssertEqual(seen.detached.count, 20)
    }

    func testSigningOutClosesTheConnectionAndTheNextUseReconnects() {
        let counts = Counts()
        let realtime = SharedChatRealtime { FakeLink(tokens: counts) }
        _ = realtime.events(cabalId: "a")

        realtime.close()
        realtime.close()
        XCTAssertEqual(counts.withLock { $0.closed }, 1)

        _ = realtime.events(cabalId: "a")
        XCTAssertEqual(counts.withLock { $0.tokens }, 2)
    }

    func testTwoSessionsOnOneCabalBothReceiveALiveMessage() async throws {
        let link = FanOutLink()
        let realtime = SharedChatRealtime { link }
        let first = try await openSession(realtime, key: "k1")
        let second = try await openSession(realtime, key: "k2")

        link.emit(.messageCreated(ChatFixtures.message("m1", minutes: 1)))

        for session in [first, second] {
            let arrived = await eventually { ChatFixtures.ids(await ChatFixtures.state(session)) == ["m1"] }
            XCTAssertTrue(arrived)
        }
        XCTAssertEqual(link.subscribed, 2)
    }

    func testASessionWhoseStreamEndedReattaches() async throws {
        let link = FanOutLink()
        let session = try await openSession(SharedChatRealtime { link }, key: "k1")

        link.endStreams()
        let reattached = await eventually {
            await session.subscribe()
            return link.subscribed == 2
        }
        XCTAssertTrue(reattached)

        link.emit(.messageCreated(ChatFixtures.message("m1", minutes: 1)))
        let arrived = await eventually { ChatFixtures.ids(await ChatFixtures.state(session)) == ["m1"] }
        XCTAssertTrue(arrived)
    }

    func testOneSessionStoppingLeavesTheOtherListening() async throws {
        let link = FanOutLink()
        let realtime = SharedChatRealtime { link }
        let first = try await openSession(realtime, key: "k1")
        let second = try await openSession(realtime, key: "k2")

        await first.close()
        let released = await eventually { link.live == 1 }
        XCTAssertTrue(released)
        link.emit(.messageCreated(ChatFixtures.message("m1", minutes: 1)))

        let arrived = await eventually { ChatFixtures.ids(await ChatFixtures.state(second)) == ["m1"] }
        XCTAssertTrue(arrived)
    }

    private func openSession(_ realtime: SharedChatRealtime, key: String) async throws -> ChatSession {
        let transport = StubTransport(scripted: [try ChatFixtures.page([])])
        let session = ChatSession(
            cabalID: ChatFixtures.cabalID,
            viewerID: ChatFixtures.viewerID,
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            realtime: realtime,
            now: { ChatFixtures.epoch },
            makeKey: { key }
        )
        await session.open()
        return session
    }

    private func eventually(_ predicate: () async -> Bool) async -> Bool {
        for _ in 0..<5000 {
            if await predicate() { return true }
            await Task.yield()
        }
        return false
    }
}

private final class FanOutLink: ChatRealtimeLink {
    private struct Store {
        var streams: [UUID: AsyncStream<ChatRealtimeEvent>.Continuation] = [:]
        var subscribed = 0
    }

    private let store = Mutex(Store())

    var subscribed: Int { store.withLock { $0.subscribed } }
    var live: Int { store.withLock { $0.streams.count } }

    func events(cabalId _: String) -> AsyncStream<ChatRealtimeEvent> {
        let (stream, continuation) = AsyncStream.makeStream(of: ChatRealtimeEvent.self)
        let id = UUID()
        store.withLock {
            $0.streams[id] = continuation
            $0.subscribed += 1
        }
        continuation.yield(.attached(resumed: true))
        continuation.onTermination = { [weak self] _ in
            _ = self?.store.withLock { $0.streams.removeValue(forKey: id) }
        }
        return stream
    }

    func emit(_ event: ChatRealtimeEvent) {
        for continuation in store.withLock({ Array($0.streams.values) }) { continuation.yield(event) }
    }

    func endStreams() {
        for continuation in store.withLock({ Array($0.streams.values) }) { continuation.finish() }
    }

    func detach(cabalId _: String) { endStreams() }
    func close() { endStreams() }
}
