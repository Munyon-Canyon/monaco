import Foundation
import HTTPTypes
import MonacoAPI
import MonacoCore
import MonacoTestClock
import MonacoTestSupport
import XCTest

@MainActor
final class StocksTabModelTests: XCTestCase {
    func testAllLoadMapsTheServerNameAndPrice() async throws {
        let transport = StubTransport(.json(.ok, Self.page(symbol: "AAPLx", name: "Apple xStock", kind: "equity")))
        let model = makeModel(transport)
        XCTAssertEqual(model.browse, .all)
        await model.load()
        XCTAssertEqual(model.rows.map(\.name), ["Apple"])
        XCTAssertEqual(model.rows.map(\.ticker), ["AAPL"])
        XCTAssertEqual(model.rows.map(\.priceText), ["$110.00"])
        XCTAssertEqual(model.phase, .loaded)
        let targets = await targets(transport)
        XCTAssertEqual(targets.count, 1)
        XCTAssertTrue(targets[0].contains("filter=all"), targets[0])
    }
    func testSearchWaitsThenSendsTheQueryAndDropsTheStalePage() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, Self.page(symbol: "AAPLx", name: "Apple xStock", kind: "equity")),
            .gate,
            .json(.ok, Self.page(symbol: "MSFTx", name: "Microsoft xStock", kind: "equity")),
        ])
        let clock = TestClock()
        let model = makeModel(transport, clock: clock)
        await model.load()
        model.setQuery("aa")
        let firstParked = await clock.state.until { $0.pending == 1 }
        XCTAssertTrue(firstParked)
        clock.advance(by: StocksTabModel.searchDebounce)
        let firstSent = await waitUntil { await self.targets(transport).count == 2 }
        XCTAssertTrue(firstSent)
        model.setQuery("app")
        let secondParked = await clock.state.until { $0.pending == 1 }
        XCTAssertTrue(secondParked)
        clock.advance(by: StocksTabModel.searchDebounce)
        let secondSent = await waitUntil { await self.targets(transport).count == 3 }
        XCTAssertTrue(secondSent)
        let arrived = await waitUntil { model.rows.map(\.ticker) == ["MSFT"] }
        XCTAssertTrue(arrived)
        await transport.releaseGate(.json(.ok, Self.page(symbol: "AAPLx", name: "Apple xStock", kind: "equity")))
        await Task.yield()
        XCTAssertEqual(model.rows.map(\.ticker), ["MSFT"])
        let targets = await targets(transport)
        XCTAssertTrue(targets[1].contains("q=aa"), targets[1])
        XCTAssertTrue(targets[2].contains("q=app"), targets[2])
    }
    func testEmptySearchReturnsToTheSelectedChip() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, Self.page(symbol: "AAPLx", name: "Apple xStock", kind: "equity")),
            .json(.ok, Self.page(symbol: "MSFTx", name: "Microsoft xStock", kind: "equity")),
            .json(.ok, Self.page(symbol: "AAPLx", name: "Apple xStock", kind: "equity")),
        ])
        let clock = TestClock()
        let model = makeModel(transport, clock: clock)
        await model.load()
        model.setQuery("ms")
        _ = await clock.state.until { $0.pending == 1 }
        clock.advance(by: StocksTabModel.searchDebounce)
        _ = await waitUntil { model.rows.map(\.ticker) == ["MSFT"] }
        model.setQuery("  ")
        let restored = await waitUntil { model.rows.map(\.ticker) == ["AAPL"] }
        XCTAssertTrue(restored)
        XCTAssertFalse(model.isSearching)
        let reloaded = await waitUntil { await self.targets(transport).count == 3 }
        XCTAssertTrue(reloaded)
        let targets = await targets(transport)
        XCTAssertTrue(targets[2].contains("filter=all") && !targets[2].contains("q="), targets.description)
    }
    func testLoadMoreAppendsTheNextCursorPage() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, Self.page(symbol: "AAPLx", name: "Apple xStock", kind: "equity", cursor: "cursor-2")),
            .json(.ok, Self.page(symbol: "TSLAx", name: "Tesla xStock", kind: "equity")),
        ])
        let model = makeModel(transport)
        await model.load()
        XCTAssertTrue(model.hasMore)
        await model.loadMore()
        XCTAssertEqual(model.all.rows.map(\.ticker), ["AAPL", "TSLA"])
        XCTAssertFalse(model.hasMore)
        let targets = await targets(transport)
        XCTAssertTrue(targets[1].contains("cursor=cursor-2"), targets[1])
    }
    func testSearchCanLoadMore() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, Self.page(symbol: "AAPLx", name: "Apple xStock", kind: "equity")),
            .json(.ok, Self.page(symbol: "MSFTx", name: "Microsoft xStock", kind: "equity", cursor: "cursor-2")),
            .json(.ok, Self.page(symbol: "TSLAx", name: "Tesla xStock", kind: "equity")),
        ])
        let clock = TestClock()
        let model = makeModel(transport, clock: clock)
        await model.load()
        model.setQuery("tech")
        _ = await clock.state.until { $0.pending == 1 }
        clock.advance(by: StocksTabModel.searchDebounce)
        let loaded = await waitUntil { model.rows.map(\.ticker) == ["MSFT"] }
        XCTAssertTrue(loaded)
        XCTAssertTrue(model.hasMore)
        await model.loadMore()
        XCTAssertEqual(model.rows.map(\.ticker), ["MSFT", "TSLA"])
        XCTAssertFalse(model.hasMore)
        let targets = await targets(transport)
        XCTAssertTrue(targets[2].contains("q=tech") && targets[2].contains("cursor=cursor-2"), targets[2])
    }
    func testUnpricedRowRendersAnEmDash() async throws {
        let transport = StubTransport(
            .json(
                .ok,
                Self.page(
                    symbol: "NEWCO",
                    name: "Newco",
                    kind: "equity",
                    price: "null",
                    change: "null",
                    spark: "null"
                )
            )
        )
        let model = makeModel(transport)
        await model.load()
        XCTAssertEqual(model.rows.map(\.priceText), ["—"])
        XCTAssertEqual(model.rows.map(\.changeText), ["—"])
        XCTAssertNil(model.rows.first?.sparkline)
    }
    func testProblemFailureSurfacesTheServerMessage() async throws {
        let transport = try StubTransport.problem(
            Components.Schemas.Problem(
                _type: .about_colon_blank,
                title: "Error",
                status: 404,
                code: .notFound,
                message: "Catalog is down.",
                traceId: "4bf92f3577b34da6a3ce929d0e0e4736",
                retryable: true
            ))
        let model = makeModel(transport)
        await model.load()
        guard case .failed(.problem(let problem)) = model.phase else {
            XCTFail("expected a problem, got \(model.phase)")
            return
        }
        XCTAssertEqual(problem.message, "Catalog is down.")
        XCTAssertEqual(ToastCopy.message(for: model.lastError ?? .decoding("missing")), "Catalog is down.")
    }
    func testResyncRefetchesAndAnotherKeyDoesNot() async throws {
        let transport = StubTransport(.json(.ok, Self.page(symbol: "AAPLx", name: "Apple xStock", kind: "equity")))
        let hints = FakeHintStream()
        let model = makeModel(transport, hints: hints)
        await model.load()
        let task = Task { await model.observe() }
        _ = await waitUntil { await hints.subscriberCount > 0 }
        addTeardownBlock { task.cancel() }
        let before = await targets(transport).count
        await hints.send(.changed(.user("user-1"), what: "balance", id: "1"))
        await hints.send(.resync)
        let refreshed = await waitUntil { await self.targets(transport).count == before + 1 }
        let after = await targets(transport).count
        XCTAssertTrue(refreshed)
        XCTAssertEqual(after, before + 1)
    }
    func testFailedRefreshKeepsTheRows() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, Self.page(symbol: "AAPLx", name: "Apple xStock", kind: "equity")),
            .failure(URLError(.cannotConnectToHost)),
        ])
        let model = makeModel(transport)
        await model.load()
        await model.load()
        XCTAssertEqual(model.rows.map(\.ticker), ["AAPL"])
        XCTAssertEqual(model.all.phase, .loaded)
        XCTAssertTrue(model.refreshFailed)
        XCTAssertGreaterThan(model.failureTick, 0)
    }
    private func makeModel(
        _ transport: StubTransport,
        hints: FakeHintStream = FakeHintStream(),
        clock: any Clock<Duration> = TestClock()
    ) -> StocksTabModel {
        StocksTabModel(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            hints: hints,
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
        continuous: Bool = false,
        price: String = "110000000",
        change: String = "1000",
        spark: String = "[100000000,110000000]",
        cursor: String? = nil
    ) -> String {
        let next = cursor.map { "\"\($0)\"" } ?? "null"
        let session = """
            {"state":"open","continuous":\(continuous),"holiday":"","early_close":false,"next_state":null,"next_transition":null}
            """
        let asset = """
            {"symbol":"\(symbol)","display_name":"\(name)","issuer":"\(issuer)","kind":"\(kind)","logo_url":null,"price_micros":\(price),"price_as_of":null,"change_bps":\(change),"sparkline_micros":\(spark),"session":\(session)}
            """
        return #"{"assets":[\#(asset)],"next_cursor":\#(next)}"#
    }
}

extension StocksTabModelTests {
    func testEachChipAsksForItsOwnFilter() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, Self.page(symbol: "AAPLx", name: "Apple xStock", kind: "equity")),
            .json(.ok, Self.page(symbol: "NVDAx", name: "NVIDIA xStock", kind: "equity")),
            .json(
                .ok,
                Self.page(symbol: "tSpaceX", name: "T-SpaceX", kind: "pre_ipo", issuer: "tessera", continuous: true)),
        ])
        let model = makeModel(transport)
        XCTAssertEqual(StocksTabModel.Browse.allCases.map(\.title), ["All", "Popular", "Pre-IPO"])
        await model.load()
        await model.show(.popular)
        XCTAssertEqual(model.rows.map(\.ticker), ["NVDA"])
        await model.show(.preIpo)
        XCTAssertEqual(model.rows.map(\.name), ["SpaceX"])
        XCTAssertEqual(model.rows.map(\.kind), [.preIpo])
        XCTAssertFalse(model.rows[0].showsSessionChip)
        let targets = await targets(transport)
        XCTAssertEqual(targets.count, 3)
        XCTAssertTrue(targets[0].contains("filter=all"), targets.description)
        XCTAssertTrue(targets[1].contains("filter=popular"), targets.description)
        XCTAssertTrue(targets[2].contains("filter=pre_ipo"), targets.description)
    }
    func testSearchWithPreIpoSelectedSendsTheQueryAndTheFilter() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, Self.page(symbol: "AAPLx", name: "Apple xStock", kind: "equity")),
            .json(.ok, Self.page(symbol: "tSpaceX", name: "T-SpaceX", kind: "pre_ipo", issuer: "tessera")),
            .json(.ok, Self.page(symbol: "tOpenAI", name: "T-OpenAI", kind: "pre_ipo", issuer: "tessera")),
            .json(.ok, Self.page(symbol: "NVDAx", name: "NVIDIA xStock", kind: "equity")),
        ])
        let clock = TestClock()
        let model = makeModel(transport, clock: clock)
        await model.load()
        await model.show(.preIpo)
        model.setQuery("open")
        _ = await clock.state.until { $0.pending == 1 }
        clock.advance(by: StocksTabModel.searchDebounce)
        let arrived = await waitUntil { model.rows.map(\.name) == ["OpenAI"] }
        XCTAssertTrue(arrived)
        XCTAssertEqual(model.browse, .preIpo)
        await model.show(.popular)
        let switched = await waitUntil { model.rows.map(\.ticker) == ["NVDA"] }
        XCTAssertTrue(switched)
        XCTAssertEqual(model.query, "open")
        let targets = await targets(transport)
        XCTAssertTrue(targets[2].contains("q=open") && targets[2].contains("filter=pre_ipo"), targets[2])
        XCTAssertTrue(targets[3].contains("q=open") && targets[3].contains("filter=popular"), targets[3])
    }
    func testStaleBrowseReplacementDoesNotBlockSearchPagination() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, Self.page(symbol: "AAPLx", name: "Apple xStock", kind: "equity")),
            .gate,
            .json(.ok, Self.page(symbol: "MSFTx", name: "Microsoft xStock", kind: "equity", cursor: "cursor-2")),
            .json(.ok, Self.page(symbol: "TSLAx", name: "Tesla xStock", kind: "equity")),
        ])
        let clock = TestClock()
        let model = makeModel(transport, clock: clock)
        await model.load()
        let stale = Task { await model.load() }
        let staleStarted = await waitUntil { await self.targets(transport).count == 2 }
        XCTAssertTrue(staleStarted)
        model.setQuery("tech")
        _ = await clock.state.until { $0.pending == 1 }
        clock.advance(by: StocksTabModel.searchDebounce)
        let searchLoaded = await waitUntil { model.rows.map(\.ticker) == ["MSFT"] }
        XCTAssertTrue(searchLoaded)
        await model.loadMore()
        XCTAssertEqual(model.rows.map(\.ticker), ["MSFT", "TSLA"])
        await transport.releaseGate(.json(.ok, Self.page(symbol: "AAPLx", name: "Apple xStock", kind: "equity")))
        await stale.value
    }
}
