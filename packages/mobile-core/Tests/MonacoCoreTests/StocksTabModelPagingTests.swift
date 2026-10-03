import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestClock
import MonacoTestSupport
import XCTest

@MainActor
final class StocksTabModelPagingTests: XCTestCase {
    func testAReplacementDropsTheOldCursorUntilItsOwnPageArrives() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, Self.page(symbol: "AAPLx", name: "Apple xStock", kind: "equity", cursor: "cursor-2")),
            .gate,
            .failure(URLError(.cannotConnectToHost)),
        ])
        let clock = TestClock()
        let model = makeModel(transport, clock: clock)
        await model.load()
        model.setQuery("ts")
        XCTAssertNil(model.nextCursor)
        await model.loadMore()
        let whileTyping = await targets(transport)
        XCTAssertEqual(whileTyping.count, 1)
        _ = await clock.state.until { $0.pending == 1 }
        clock.advance(by: StocksTabModel.searchDebounce)
        let searching = await waitUntil { await self.targets(transport).count == 2 }
        XCTAssertTrue(searching)
        XCTAssertNil(model.nextCursor)
        await model.loadMore()
        let duringSearch = await targets(transport)
        XCTAssertEqual(duringSearch.count, 2)
        XCTAssertFalse(duringSearch[1].contains("cursor="), duringSearch[1])
        await transport.releaseGate(
            .json(.ok, Self.page(symbol: "TSLAx", name: "Tesla xStock", kind: "equity", cursor: "cursor-3")))
        let arrived = await waitUntil { model.nextCursor == "cursor-3" }
        XCTAssertTrue(arrived)
        model.setQuery("nope")
        XCTAssertTrue(model.rows.isEmpty)
        XCTAssertEqual(model.phase, .loading)
        _ = await clock.state.until { $0.pending == 1 }
        clock.advance(by: StocksTabModel.searchDebounce)
        let failed = await waitUntil {
            if case .failed = model.phase { return true }
            return false
        }
        XCTAssertTrue(failed)
        XCTAssertTrue(model.rows.isEmpty)
        XCTAssertNil(model.nextCursor)
        await model.loadMore()
        let sent = await targets(transport)
        XCTAssertFalse(sent.contains { $0.contains("cursor=") })
    }
    func testRefreshDropsTheCursorUntilTheFirstPageReturns() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, Self.page(symbol: "AAPLx", name: "Apple xStock", kind: "equity", cursor: "cursor-2")),
            .json(.ok, Self.page(symbol: "TSLAx", name: "Tesla xStock", kind: "equity", cursor: "cursor-3")),
            .gate,
        ])
        let model = makeModel(transport)
        await model.load()
        await model.loadMore()
        XCTAssertEqual(model.rows.map(\.ticker), ["AAPL", "TSLA"])
        let refreshing = Task { await model.load() }
        let sent = await waitUntil { await self.targets(transport).count == 3 }
        XCTAssertTrue(sent)
        XCTAssertNil(model.nextCursor)
        await model.loadMore()
        let during = await targets(transport)
        XCTAssertEqual(during.count, 3)
        XCTAssertFalse(during[2].contains("cursor="), during[2])
        await transport.releaseGate(
            .json(.ok, Self.page(symbol: "AAPLx", name: "Apple xStock", kind: "equity", cursor: "cursor-2")))
        await refreshing.value
        XCTAssertEqual(model.rows.map(\.ticker), ["AAPL"])
        XCTAssertEqual(model.nextCursor, "cursor-2")
    }
    func testAFailedRefreshRestoresTheCursor() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, Self.page(symbol: "AAPLx", name: "Apple xStock", kind: "equity", cursor: "cursor-2")),
            .failure(URLError(.cannotConnectToHost)),
            .json(.ok, Self.page(symbol: "TSLAx", name: "Tesla xStock", kind: "equity")),
        ])
        let model = makeModel(transport)
        await model.load()
        await model.load()
        XCTAssertEqual(model.rows.map(\.ticker), ["AAPL"])
        XCTAssertTrue(model.refreshFailed)
        XCTAssertEqual(model.nextCursor, "cursor-2")
        await model.loadMore()
        XCTAssertEqual(model.rows.map(\.ticker), ["AAPL", "TSLA"])
        let targets = await targets(transport)
        XCTAssertTrue(targets[2].contains("cursor=cursor-2"), targets[2])
    }
    func testPriceHintsKeepLoadedPagesAndTheirCursor() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, Self.page(symbol: "AAPLx", name: "Apple xStock", kind: "equity", cursor: "cursor-2")),
            .json(.ok, Self.page(symbol: "TSLAx", name: "Tesla xStock", kind: "equity", cursor: "cursor-3")),
            .json(.ok, Self.page(symbol: "AAPLx", name: "Apple xStock", kind: "equity", cursor: "cursor-2")),
            .json(.ok, Self.page(symbol: "TSLAx", name: "Tesla xStock", kind: "equity", cursor: "cursor-3")),
        ])
        let hints = FakeHintStream()
        let model = StocksTabModel(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            hints: hints,
            clock: TestClock()
        )
        await model.load()
        await model.loadMore()
        let task = Task { await model.observe() }
        _ = await waitUntil { await hints.subscriberCount > 0 }
        addTeardownBlock { task.cancel() }
        await hints.send(.changed(.global, what: "prices_updated", id: "1"))
        let requested = await waitUntil { await self.targets(transport).count == 4 }
        XCTAssertTrue(requested)
        let refreshed = await waitUntil { model.rows.map(\.ticker) == ["AAPL", "TSLA"] }
        XCTAssertTrue(refreshed)
        XCTAssertEqual(model.nextCursor, "cursor-3")
    }
    func testHintDuringSearchDebounceRefreshesBothSearchPagesLater() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, Self.page(symbol: "AAPLx", name: "Apple xStock", kind: "equity", cursor: "cursor-2")),
            .json(.ok, Self.page(symbol: "TSLAx", name: "Tesla xStock", kind: "equity", cursor: "cursor-2")),
            .json(.ok, Self.page(symbol: "NVDAx", name: "NVIDIA xStock", kind: "equity", cursor: "cursor-3")),
            .json(.ok, Self.page(symbol: "TSLAx", name: "Tesla xStock", kind: "equity", cursor: "cursor-2")),
            .json(.ok, Self.page(symbol: "NVDAx", name: "NVIDIA xStock", kind: "equity", cursor: "cursor-3")),
        ])
        let hints = FakeHintStream()
        let clock = TestClock()
        let model = StocksTabModel(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            hints: hints,
            clock: clock
        )
        await model.load()
        let task = Task { await model.observe() }
        _ = await waitUntil { await hints.subscriberCount > 0 }
        addTeardownBlock { task.cancel() }
        model.setQuery("ts")
        _ = await clock.state.until { $0.pending == 1 }
        await hints.send(.changed(.global, what: "prices_updated", id: "1"))
        let firstPage = await waitUntil { model.rows.map(\.ticker) == ["TSLA"] }
        XCTAssertTrue(firstPage)
        clock.advance(by: StocksTabModel.searchDebounce)
        await Task.yield()
        await model.loadMore()
        XCTAssertEqual(model.rows.map(\.ticker), ["TSLA", "NVDA"])
        await hints.send(.changed(.global, what: "prices_updated", id: "2"))
        let refreshed = await waitUntil { await self.targets(transport).count == 5 }
        XCTAssertTrue(refreshed)
        XCTAssertEqual(model.rows.map(\.ticker), ["TSLA", "NVDA"])
        XCTAssertEqual(model.nextCursor, "cursor-3")
        let sent = await targets(transport)
        XCTAssertTrue(sent[3].contains("q=ts"), sent[3])
        XCTAssertFalse(sent[3].contains("cursor="), sent[3])
        XCTAssertTrue(sent[4].contains("q=ts") && sent[4].contains("cursor=cursor-2"), sent[4])
    }
    func testAnOverlappingFailedRefreshKeepsTheRowsCursor() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, Self.page(symbol: "AAPLx", name: "Apple xStock", kind: "equity", cursor: "cursor-2")),
            .gate,
            .failure(URLError(.cannotConnectToHost)),
            .json(.ok, Self.page(symbol: "TSLAx", name: "Tesla xStock", kind: "equity")),
        ])
        let model = makeModel(transport)
        await model.load()
        let first = Task { await model.load() }
        let gated = await waitUntil { await self.targets(transport).count == 2 }
        XCTAssertTrue(gated)
        XCTAssertNil(model.nextCursor)
        let second = Task { await model.load() }
        let failed = await waitUntil { model.refreshFailed }
        XCTAssertTrue(failed)
        await second.value
        XCTAssertEqual(model.rows.map(\.ticker), ["AAPL"])
        XCTAssertEqual(model.nextCursor, "cursor-2")
        await model.loadMore()
        XCTAssertEqual(model.rows.map(\.ticker), ["AAPL", "TSLA"])
        let sent = await targets(transport)
        XCTAssertTrue(sent[3].contains("cursor=cursor-2"), sent[3])
        await transport.releaseGate(
            .json(.ok, Self.page(symbol: "NVDAx", name: "NVIDIA xStock", kind: "equity", cursor: "cursor-9")))
        await first.value
        XCTAssertEqual(model.rows.map(\.ticker), ["AAPL", "TSLA"])
        XCTAssertNil(model.nextCursor)
    }
    func testSwitchingBrowseDropsTheOldCursor() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, Self.page(symbol: "AAPLx", name: "Apple xStock", kind: "equity", cursor: "cursor-2")),
            .gate,
        ])
        let model = makeModel(transport)
        await model.load()
        let switching = Task { await model.show(.preIpo) }
        let sent = await waitUntil { await self.targets(transport).count == 2 }
        XCTAssertTrue(sent)
        XCTAssertNil(model.nextCursor)
        await model.loadMore()
        let targets = await targets(transport)
        XCTAssertEqual(targets.count, 2)
        XCTAssertTrue(targets[1].contains("filter=pre_ipo"), targets[1])
        XCTAssertFalse(targets[1].contains("cursor="), targets[1])
        await transport.releaseGate(
            .json(.ok, Self.page(symbol: "tSpaceX", name: "T-SpaceX", kind: "pre_ipo", issuer: "tessera")))
        await switching.value
        XCTAssertEqual(model.rows.map(\.name), ["SpaceX"])
        XCTAssertNil(model.nextCursor)
    }
    private func makeModel(
        _ transport: StubTransport,
        clock: any Clock<Duration> = TestClock()
    ) -> StocksTabModel {
        StocksTabModel(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            hints: FakeHintStream(),
            clock: clock
        )
    }
    private func targets(_ transport: StubTransport) async -> [String] {
        await transport.sent.map { $0.path ?? "" }
    }
    private func waitUntil(_ predicate: @escaping () async -> Bool) async -> Bool {
        for _ in 0..<500 {
            if await predicate() { return true }
            await Task.yield()
        }
        return await predicate()
    }
    private static func page(
        symbol: String,
        name: String,
        kind: String,
        issuer: String = "xstocks",
        price: String = "110000000",
        change: String = "1000",
        spark: String = "[100000000,110000000]",
        cursor: String? = nil
    ) -> String {
        let next = cursor.map { "\"\($0)\"" } ?? "null"
        let session = """
            {"state":"open","continuous":false,"holiday":"","early_close":false,"next_state":null,"next_transition":null}
            """
        let asset = """
            {"symbol":"\(symbol)","display_name":"\(name)","issuer":"\(issuer)","kind":"\(kind)","logo_url":null,"price_micros":\(price),"price_as_of":null,"change_bps":\(change),"sparkline_micros":\(spark),"session":\(session)}
            """
        return #"{"assets":[\#(asset)],"next_cursor":\#(next)}"#
    }
}
