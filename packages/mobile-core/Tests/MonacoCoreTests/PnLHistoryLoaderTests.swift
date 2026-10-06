import Foundation
import MonacoAPI
import MonacoTestSupport
import XCTest

@testable import MonacoCore

final class PnLHistoryLoaderTests: XCTestCase {
    private typealias MyHistory = Components.Schemas.MyPnlHistory
    private typealias PotHistory = Components.Schemas.CabalValueHistory

    private static let cabalID = Components.Schemas.CabalRef.sampleAlpha.id

    func testACurveIsCachedPerSubjectAndRange() async throws {
        let (loader, transport, _) = try make([mine(._1d), mine(._1w)])

        let day = try await loader.show(.me, range: .oneDay)
        let again = try await loader.show(.me, range: .oneDay)
        let week = try await loader.show(.me, range: .oneWeek)

        XCTAssertEqual(day, again)
        XCTAssertNotNil(week)
        let paths = await transport.sent.map(\.path)
        XCTAssertEqual(paths, ["/v1/me/pnl-history?range=1D", "/v1/me/pnl-history?range=1W"])
        let cached = await loader.cached(.me, range: .oneDay)
        XCTAssertEqual(cached, day)
    }

    func testACabalReadsItsValueHistory() async throws {
        let (loader, transport, _) = try make([pot(._1m)])

        let curve = try await loader.show(.cabal(id: Self.cabalID), range: .oneMonth)

        XCTAssertEqual(curve, ValueCurve(PotHistory.sample()))
        let paths = await transport.sent.map(\.path)
        XCTAssertEqual(paths, ["/v1/cabals/\(Self.cabalID)/value-history?range=1M"])
    }

    func testSeveralCabalsLoadTogetherAtOneRange() async throws {
        let (loader, transport, _) = try make([pot(._1m), pot(._1m)])
        let other = Components.Schemas.CabalRef.sampleBeta.id

        let curves = try await loader.show([.cabal(id: Self.cabalID), .cabal(id: other)], range: .oneMonth)

        XCTAssertEqual(curves?.count, 2)
        let count = await transport.sent.count
        XCTAssertEqual(count, 2)
    }

    func testRapidRangeChangesSendOnlyTheFirstAndTheLatestRequest() async throws {
        let transport = StubTransport(scripted: [.gate, try mine(._1w)])
        let loader = PnLHistoryLoader(api: api(transport), hints: FakeHintStream())

        async let hour = loader.show(.me, range: .oneHour)
        await transport.waitForRequest()
        async let day = loader.show(.me, range: .oneDay)
        await settle(loader, on: .oneDay)
        async let week = loader.show(.me, range: .oneWeek)
        await settle(loader, on: .oneWeek)
        await transport.releaseGate(try mine(._1h))

        let results = try await (hour, day, week)

        XCTAssertNil(results.0)
        XCTAssertNil(results.1)
        XCTAssertEqual(results.2, ValueCurve(MyHistory.sample(range: ._1w)))
        let paths = await transport.sent.map(\.path)
        XCTAssertEqual(paths, ["/v1/me/pnl-history?range=1H", "/v1/me/pnl-history?range=1W"])
        let overtaken = await loader.cached(.me, range: .oneHour)
        XCTAssertNotNil(overtaken)
    }

    func testAUserHintDropsTheCacheAndRefetchesOnlyTheVisibleSeries() async throws {
        let (loader, transport, hints) = try make([mine(._1d), mine(._1w), mine(._1w)])
        _ = try await loader.show(.me, range: .oneDay)
        _ = try await loader.show(.me, range: .oneWeek)
        let updates = Updates()
        let task = Task { await loader.observe { await updates.add($0) } }
        await subscribed(hints)

        await hints.send(.changed(.user("u1"), what: "balance", id: "1"))
        await transport.waitForRequests(3)
        await updates.reaches(1)

        let paths = await transport.sent.map(\.path)
        XCTAssertEqual(paths.last, "/v1/me/pnl-history?range=1W")
        let dropped = await loader.cached(.me, range: .oneDay)
        XCTAssertNil(dropped)
        guard case .refreshed(let range, let curves) = await updates.all[0] else { return XCTFail("want a refresh") }
        XCTAssertEqual(range, .oneWeek)
        XCTAssertNotNil(curves[.me])
        task.cancel()
    }

    func testALeaderboardsUpdatedHintRefetchesAndAnotherGlobalHintDoesNot() async throws {
        let (loader, transport, hints) = try make([mine(._1d), mine(._1d)])
        _ = try await loader.show(.me, range: .oneDay)
        let updates = Updates()
        let task = Task { await loader.observe { await updates.add($0) } }
        await subscribed(hints)

        await hints.send(.changed(.global, what: "maintenance", id: "1"))
        await hints.send(.changed(.global, what: "leaderboards_updated", id: "2"))
        await updates.reaches(1)

        let count = await transport.sent.count
        XCTAssertEqual(count, 2)
        task.cancel()
    }

    func testAResyncRefetchesOnceEvenThoughBothStreamsGetIt() async throws {
        let (loader, transport, hints) = try make([mine(._1d), mine(._1d)])
        _ = try await loader.show(.me, range: .oneDay)
        let updates = Updates()
        let task = Task { await loader.observe { await updates.add($0) } }
        await subscribed(hints)

        await hints.send(.resync)
        await updates.reaches(1)
        for _ in 0..<50 { await Task.yield() }

        let count = await transport.sent.count
        XCTAssertEqual(count, 2)
        task.cancel()
    }

    func testAHiddenScreenHoldsTheRefetchUntilItShowsAgain() async throws {
        let (loader, transport, hints) = try make([mine(._1d), mine(._1d)])
        _ = try await loader.show(.me, range: .oneDay)
        let updates = Updates()
        let task = Task { await loader.observe { await updates.add($0) } }
        await subscribed(hints)
        await loader.setVisible(false)

        await hints.send(.changed(.user("u1"), what: "balance", id: "1"))
        for _ in 0..<50 { await Task.yield() }
        let held = await transport.sent.count
        await loader.setVisible(true)
        await updates.reaches(1)

        XCTAssertEqual(held, 1)
        let count = await transport.sent.count
        XCTAssertEqual(count, 2)
        task.cancel()
    }

    func testAFailedRefetchKeepsTheScreenAndReportsTheError() async throws {
        let transport = StubTransport(scripted: [try mine(._1d), .failure(URLError(.notConnectedToInternet))])
        let hints = FakeHintStream()
        let loader = PnLHistoryLoader(api: api(transport), hints: hints)
        _ = try await loader.show(.me, range: .oneDay)
        let updates = Updates()
        let task = Task { await loader.observe { await updates.add($0) } }
        await subscribed(hints)

        await hints.send(.changed(.user("u1"), what: "balance", id: "1"))
        await updates.reaches(1)

        guard case .failed(.transport) = await updates.all[0] else { return XCTFail("want a transport failure") }
        task.cancel()
    }

    func testAFailedFirstLoadThrowsAnAPIError() async throws {
        let transport = StubTransport(scripted: [.failure(URLError(.notConnectedToInternet))])
        let loader = PnLHistoryLoader(api: api(transport), hints: FakeHintStream())

        do {
            _ = try await loader.show(.me, range: .oneDay)
            XCTFail("want a failure")
        } catch let error as APIError {
            guard case .transport = error else { return XCTFail("want a transport failure, got \(error)") }
        }
    }

    private actor Updates {
        private(set) var all: [PnLHistoryLoader.Update] = []

        private var waiters: [(count: Int, continuation: CheckedContinuation<Void, Never>)] = []

        func add(_ update: PnLHistoryLoader.Update) {
            all.append(update)
            let ready = waiters.filter { $0.count <= all.count }
            waiters.removeAll { $0.count <= all.count }
            for waiter in ready { waiter.continuation.resume() }
        }

        func reaches(_ count: Int) async {
            while all.count < count { await Task.yield() }
        }
    }

    private func make(_ replies: [StubTransport.Reply]) throws -> (PnLHistoryLoader, StubTransport, FakeHintStream) {
        let transport = StubTransport(scripted: replies)
        let hints = FakeHintStream()
        return (PnLHistoryLoader(api: api(transport), hints: hints), transport, hints)
    }

    private func api(_ transport: StubTransport) -> APIClient {
        APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
    }

    private func mine(_ range: MyHistory.RangePayload) throws -> StubTransport.Reply {
        .json(.ok, try encode(MyHistory.sample(range: range)))
    }

    private func pot(_ range: PotHistory.RangePayload) throws -> StubTransport.Reply {
        .json(.ok, try encode(PotHistory.sample(range: range)))
    }

    private func encode(_ value: some Encodable) throws -> String {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return String(decoding: try encoder.encode(value), as: UTF8.self)
    }

    private func settle(_ loader: PnLHistoryLoader, on range: LeaderboardRange) async {
        while await loader.visibleRange != range { await Task.yield() }
    }

    private func subscribed(_ hints: FakeHintStream) async {
        while await hints.subscriberCount < 2 { await Task.yield() }
    }
}
