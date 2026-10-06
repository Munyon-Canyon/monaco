import Foundation
import MonacoCore
import Synchronization
import XCTest

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
}
