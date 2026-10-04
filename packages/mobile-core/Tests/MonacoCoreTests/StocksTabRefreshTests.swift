import MonacoAPI
import MonacoCore
import MonacoTestClock
import MonacoTestSupport
import XCTest

@MainActor
final class StocksTabRefreshTests: XCTestCase {
    func testPriceHintsAndResyncRefreshButOtherGlobalHintsDoNot() async {
        let transport = StubTransport(scripted: [
            .json(.ok, Self.page(symbols: ["AAPLx"], price: "100000000", change: "1000", cursor: nil)),
            .json(.ok, Self.page(symbols: ["AAPLx"], price: "200000000", change: "2000", cursor: nil)),
            .json(.ok, Self.page(symbols: ["AAPLx"], price: "300000000", change: "3000", cursor: nil)),
        ])
        let hints = FakeHintStream()
        let model = StocksTabModel(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            hints: hints,
            clock: TestClock()
        )
        await model.load()
        let observer = Task { await model.observe() }
        addTeardownBlock { observer.cancel() }
        let subscribed = await waitUntil { await hints.subscriberCount == 1 }
        XCTAssertTrue(subscribed)

        await hints.send(.changed(.global, what: "prices_updated", id: "1"))
        let priceRefreshed = await waitUntil { model.rows.first?.priceMicros == 200_000_000 }
        let priceRequests = await transport.sent.count
        XCTAssertTrue(priceRefreshed)
        XCTAssertEqual(priceRequests, 2)

        await hints.send(.resync)
        let resynced = await waitUntil { model.rows.first?.priceMicros == 300_000_000 }
        let resyncRequests = await transport.sent.count
        XCTAssertTrue(resynced)
        XCTAssertEqual(resyncRequests, 3)

        await hints.send(.changed(.global, what: "other", id: "2"))
        for _ in 0..<10 { await Task.yield() }
        let otherRequests = await transport.sent.count
        XCTAssertEqual(otherRequests, 3)
    }

    func testPriceRefreshPreventsPaginationUntilItsPagesFinish() async {
        let transport = StubTransport(scripted: [
            .json(.ok, Self.page(symbols: ["AAPLx"], price: "100000000", change: "1000", cursor: "page-2")),
            .gate,
        ])
        let hints = FakeHintStream()
        let model = StocksTabModel(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            hints: hints,
            clock: TestClock()
        )
        await model.load()
        let observer = Task { await model.observe() }
        addTeardownBlock { observer.cancel() }
        _ = await waitUntil { await hints.subscriberCount == 1 }

        await hints.send(.changed(.global, what: "prices_updated", id: "1"))
        let refreshStarted = await waitUntil { await transport.sent.count == 2 }
        XCTAssertTrue(refreshStarted)
        await model.loadMore()
        let requestsDuringRefresh = await transport.sent.count
        XCTAssertEqual(requestsDuringRefresh, 2)

        await transport.releaseGate(
            .json(.ok, Self.page(symbols: ["AAPLx"], price: "200000000", change: "2000", cursor: "page-2")))
        let refreshed = await waitUntil { model.rows.first?.priceMicros == 200_000_000 }
        XCTAssertTrue(refreshed)
        XCTAssertEqual(model.rows.map(\.ticker), ["AAPL"])
        XCTAssertEqual(model.nextCursor, "page-2")
    }

    func testPriceHintsRefreshEveryLoadedPagePastTwentyRows() async throws {
        let firstPage = (1...20).map { "S\($0)x" }
        let transport = StubTransport(scripted: [
            .json(.ok, Self.page(symbols: firstPage, price: "100000000", change: "1000", cursor: "page-2")),
            .json(.ok, Self.page(symbols: ["LASTx"], price: "100000000", change: "1000", cursor: nil)),
            .json(.ok, Self.page(symbols: firstPage, price: "200000000", change: "2000", cursor: "page-2")),
            .json(.ok, Self.page(symbols: ["LASTx"], price: "200000000", change: "2000", cursor: nil)),
        ])
        let hints = FakeHintStream()
        let model = StocksTabModel(
            api: APIClient(
                serverURL: testServerURL,
                tokens: StubTokenProvider(token: "token-1"),
                transport: transport
            ),
            hints: hints,
            clock: TestClock()
        )

        await model.load()
        await model.loadMore()
        XCTAssertEqual(model.rows.count, 21)

        let observer = Task { await model.observe() }
        addTeardownBlock { observer.cancel() }
        let subscribed = await waitUntil { await hints.subscriberCount == 1 }
        XCTAssertTrue(subscribed)

        await hints.send(.changed(.global, what: "prices_updated", id: "1"))
        let refreshed = await waitUntil { model.rows.last?.priceMicros == 200_000_000 }
        XCTAssertTrue(refreshed)

        let laterPage = try XCTUnwrap(model.rows.last)
        XCTAssertEqual(laterPage.changeBasisPoints, 2000)
        XCTAssertEqual(laterPage.sparkline?.heights.last, 1)
        let requests = await transport.sent.count
        XCTAssertEqual(requests, 4)
    }

    private func waitUntil(_ predicate: @escaping () async -> Bool) async -> Bool {
        for _ in 0..<500 {
            if await predicate() { return true }
            await Task.yield()
        }
        return await predicate()
    }

    private static func page(symbols: [String], price: String, change: String, cursor: String?) -> String {
        let assets = symbols.map { symbol in
            """
            {"symbol":"\(symbol)","display_name":"\(symbol) Stock","issuer":"xstocks","kind":"equity","logo_url":null,"price_micros":\(price),"price_as_of":null,"change_bps":\(change),"sparkline_micros":[100000000,\(price)],"session":{"state":"open","continuous":false,"holiday":"","early_close":false,"next_state":null,"next_transition":null}}
            """
        }.joined(separator: ",")
        let next = cursor.map { "\"\($0)\"" } ?? "null"
        return #"{"assets":[\#(assets)],"next_cursor":\#(next)}"#
    }
}
