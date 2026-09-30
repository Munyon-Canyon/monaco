import Foundation
import MonacoTestSupport
import XCTest

@testable import MonacoAPI

final class HintStreamTests: XCTestCase {
    private let clock = TestClock()

    private func makeStream(
        _ transport: FakeStreamTransport,
        token: @escaping @Sendable () async throws -> String? = { "token" },
        refresh: @escaping @Sendable (String) async throws -> String? = { _ in nil },
        random: @escaping @Sendable () -> Double = { 0 }
    ) -> HintStream {
        HintStream(
            serverURL: testServerURL,
            transport: transport,
            token: token,
            refresh: refresh,
            clock: clock,
            random: random
        )
    }

    private func eventually(_ condition: () async -> Bool) async -> Bool {
        let deadline = ContinuousClock.now + .seconds(1)
        while ContinuousClock.now < deadline {
            if await condition() { return true }
            await Task.yield()
        }
        return false
    }

    private func hint(_ id: Int, _ key: String, _ what: String) -> String {
        "id: \(id)\nevent: hint\ndata: {\"key\":\"\(key)\",\"what\":\"\(what)\"}\n\n"
    }

    func testReconnectSendsLastEventID() async throws {
        let transport = FakeStreamTransport(clock: clock)
        let stream = makeStream(transport)
        let hints = Collector(stream.hints(matching: .global(what: nil)))
        await stream.start()

        let first = try await transport.connection(1)
        first.send(hint(5, "global", "feed"))
        _ = await hints.first(2)
        first.close()

        let requests = try await transport.requests(2)
        XCTAssertEqual(requests.map(\.lastEventID), [nil, "5"])
        await stream.stop()
    }

    func testReconnectEmitsResync() async throws {
        let transport = FakeStreamTransport(clock: clock)
        let stream = makeStream(transport)
        let hints = Collector(stream.hints(matching: .cabal(id: "42", what: nil)))
        await stream.start()

        let first = try await transport.connection(1)
        first.send(hint(1, "cabal:42", "updated"))
        _ = await hints.first(2)
        first.close()
        let second = try await transport.connection(2)
        second.send("event: resync\ndata: {}\n\n")
        second.send(hint(1, "cabal:42", "voted"))

        let received = await hints.first(4)
        XCTAssertEqual(
            received,
            [
                .resync,
                .changed(.cabal("42"), what: "updated", id: "1"),
                .resync,
                .changed(.cabal("42"), what: "voted", id: "1"),
            ])
        await stream.stop()
    }

    func testResyncReachesEverySubscriber() async throws {
        let transport = FakeStreamTransport(clock: clock)
        let stream = makeStream(transport)
        let user = Collector(stream.hints(matching: .user(what: "balance")))
        let cabal = Collector(stream.hints(matching: .cabal(id: "7", what: nil)))
        let global = Collector(stream.hints(matching: .global(what: "feed")))
        await stream.start()

        let connection = try await transport.connection(1)
        connection.send(hint(1, "global", "feed"))

        let globalHints = await global.first(2)
        let userHints = await user.first(1)
        let cabalHints = await cabal.first(1)
        XCTAssertEqual(globalHints, [.resync, .changed(.global, what: "feed", id: "1")])
        XCTAssertEqual(userHints, [.resync])
        XCTAssertEqual(cabalHints, [.resync])
        await stream.stop()
    }

    func testMalformedHintsAreDroppedAndCounted() async throws {
        let transport = FakeStreamTransport(clock: clock)
        let stream = makeStream(transport)
        let hints = Collector(stream.hints(matching: .cabal(id: "42", what: nil)))
        await stream.start()

        let connection = try await transport.connection(1)
        connection.send(hint(1, "cabal:", "updated"))
        connection.send(hint(2, "cabal:42", ""))
        connection.send(hint(3, "cabal:42", "Updated"))
        connection.send("id: 4\nevent: hint\ndata: not json\n\n")
        connection.send(hint(5, "cabal:42", "updated"))

        let received = await hints.first(2)
        XCTAssertEqual(received, [.resync, .changed(.cabal("42"), what: "updated", id: "5")])
        let dropped = await stream.droppedCount
        XCTAssertEqual(dropped, 4)
        await stream.stop()
    }

    func testBackoffSchedule() async throws {
        let transport = FakeStreamTransport(then: .unreachable, clock: clock)
        let jitter: [Double] = [0.5, 0.25, 0.999, 0.1, 0.75, 0.6, 0.999, 0.3]
        let draws = Watched(jitter)
        let stream = makeStream(transport, random: { draws.mutate { $0.isEmpty ? 0.5 : $0.removeFirst() } })
        let isBackoff: (Duration) -> Bool = { $0 != HintStream.fallbackInterval }
        await stream.start()

        for attempt in jitter.indices {
            let parked = await clock.state.until { $0.requested.filter(isBackoff).count > attempt }
            XCTAssertTrue(parked, "attempt \(attempt + 1) never backed off")
            clock.advance(by: clock.state.current.requested.filter(isBackoff)[attempt])
        }
        try await transport.requests(jitter.count + 1)

        let ceilings: [Duration] = [1, 2, 4, 8, 16, 30, 30, 30].map { .seconds($0) }
        let backoffs = Array(clock.state.current.requested.filter(isBackoff).prefix(jitter.count))
        XCTAssertEqual(backoffs, zip(ceilings, jitter).map { $0 * $1 })
        for (backoff, ceiling) in zip(backoffs, ceilings) {
            XCTAssertTrue(backoff >= .zero && backoff < ceiling, "\(backoff) outside 0..<\(ceiling)")
        }
        await stream.stop()
    }

    func testBackoffResetsAfterAMinuteConnected() async throws {
        let transport = FakeStreamTransport(
            [.unreachable, .stream, .unreachable, .stream], then: .unreachable, clock: clock)
        let stream = makeStream(transport, random: { 0.5 })
        let hints = Collector(stream.hints(matching: .global(what: nil)))
        let halfCeilings: [Duration] = [.milliseconds(500), .seconds(1), .seconds(2), .seconds(4)]
        let backoffs = { (state: TestClock.State) in state.requested.filter(halfCeilings.contains) }
        let heartbeats = { (state: TestClock.State) in
            state.requested.filter { $0 == HintStream.heartbeatTimeout }.count
        }
        await stream.start()

        _ = await clock.state.until { backoffs($0).count == 1 }
        clock.advance(by: .milliseconds(500))
        let short = try await transport.connection(1)
        _ = await clock.state.until { heartbeats($0) == 1 }
        clock.advance(by: .seconds(10))
        short.close()
        _ = await clock.state.until { backoffs($0).count == 2 }
        clock.advance(by: .seconds(1))
        _ = await clock.state.until { backoffs($0).count == 3 }
        clock.advance(by: .seconds(2))

        let long = try await transport.connection(2)
        _ = await clock.state.until { heartbeats($0) == 2 }
        clock.advance(by: .seconds(20))
        long.send(hint(1, "global", "feed"))
        _ = await hints.first(3)
        clock.advance(by: .seconds(25))
        _ = await clock.state.until { $0.requested.last == .seconds(20) }
        clock.advance(by: .seconds(15))
        long.close()

        _ = await clock.state.until { backoffs($0).count == 4 }
        XCTAssertEqual(
            backoffs(clock.state.current),
            [.milliseconds(500), .seconds(1), .seconds(2), .milliseconds(500)],
            "a 10 s connection keeps the backoff climbing; a 60 s one resets it to 1 s"
        )
        await stream.stop()
    }

    func testHeartbeatTimeoutReconnects() async throws {
        let transport = FakeStreamTransport(clock: clock)
        let stream = makeStream(transport)
        let hints = Collector(stream.hints(matching: .global(what: nil)))
        await stream.start()
        let connection = try await transport.connection(1)
        let armed = await clock.state.until { $0.pending == 1 && $0.requested.last == HintStream.heartbeatTimeout }
        XCTAssertTrue(armed)

        clock.advance(by: .seconds(20))
        connection.send(": heartbeat\n\n")
        connection.send(hint(1, "global", "feed"))
        _ = await hints.first(2)
        clock.advance(by: .seconds(25))
        let rearmed = await clock.state.until { $0.requested.last == .seconds(20) }
        XCTAssertTrue(rearmed, "bytes at 20 s push the timeout to 65 s")
        XCTAssertEqual(transport.state.current.requests.count, 1)
        clock.advance(by: .seconds(20))

        let requests = try await transport.requests(2)
        XCTAssertEqual(requests.map(\.at), [.zero, .seconds(65)])
        await stream.stop()
    }

    func testFirst401RefreshesAndReconnects() async throws {
        let transport = FakeStreamTransport([.unauthorized], clock: clock)
        let refreshed = Watched<[String]>([])
        let stream = makeStream(
            transport, token: { "stale" },
            refresh: { rejected in
                refreshed.mutate { $0.append(rejected) }
                return "fresh"
            })
        let hints = Collector(stream.hints(matching: .global(what: nil)))
        await stream.start()

        _ = await hints.first(1)
        let requests = try await transport.requests(2)
        XCTAssertEqual(requests.map(\.authorization), ["Bearer stale", "Bearer fresh"])
        XCTAssertEqual(refreshed.current, ["stale"])
        let state = await stream.state
        XCTAssertEqual(state, .connected)
        await stream.stop()
    }

    func testSecond401StopsSignedOut() async throws {
        let transport = FakeStreamTransport([.unauthorized, .unauthorized], clock: clock)
        let refreshed = Watched<[String]>([])
        let stream = makeStream(
            transport, token: { "stale" },
            refresh: { rejected in
                refreshed.mutate { $0.append(rejected) }
                return "fresh"
            })
        await stream.start()

        let signedOut = await eventually { await stream.state == .signedOut }
        XCTAssertTrue(signedOut)
        XCTAssertEqual(transport.state.current.requests.map(\.authorization), ["Bearer stale", "Bearer fresh"])
        XCTAssertEqual(refreshed.current, ["stale"])
        let quiet = await clock.state.until { $0.pending == 0 }
        XCTAssertTrue(quiet, "a signed-out stream keeps no timers")
        await stream.stop()
    }

    func testFallbackResyncEvery30sWhileDown() async throws {
        let transport = FakeStreamTransport(then: .hang, clock: clock)
        let stream = makeStream(transport)
        let hints = Collector(stream.hints(matching: .user(what: nil)))
        await stream.start()

        for tick in 1...3 {
            let parked = await clock.state.until { $0.pending == 1 && $0.requested.count == tick }
            XCTAssertTrue(parked)
            XCTAssertEqual(clock.state.current.requested.last, HintStream.fallbackInterval)
            clock.advance(by: HintStream.fallbackInterval)
            let received = await hints.first(tick)
            XCTAssertEqual(received, Array(repeating: .resync, count: tick))
        }
        let requests = try await transport.requests(1)
        XCTAssertEqual(requests.count, 1, "the resyncs came while the one connect attempt hung")
        await stream.stop()
    }

    func testStartAndStopAreIdempotent() async throws {
        let transport = FakeStreamTransport(clock: clock)
        let stream = makeStream(transport)
        let hints = Collector(stream.hints(matching: .global(what: nil)))
        await stream.start()
        await stream.start()
        _ = await hints.first(1)
        let connected = await stream.state
        XCTAssertEqual(connected, .connected)

        await stream.stop()
        await stream.stop()
        let stopped = await stream.state
        XCTAssertEqual(stopped, .stopped)

        await stream.start()
        _ = try await transport.connection(2)
        XCTAssertEqual(transport.state.current.requests.count, 2)
        await stream.stop()
    }
}
