import Foundation
import HTTPTypes
import MonacoAPI
import MonacoCore
import MonacoTestClock
import MonacoTestSupport
import OpenAPIRuntime
import Synchronization
import XCTest

@MainActor
final class StocksTabMarketsTests: XCTestCase {
    func testTheRailShowsTheBiggestDayMovesFirstWhetherUpOrDown() async throws {
        let model = try await popularModel([
            ("AAPLx", 120), ("NVDAx", 418), ("TSLAx", -310), ("MSFTx", 62), ("AMDx", -80),
        ])

        XCTAssertEqual(model.topMovers.map(\.ticker), ["NVDA", "TSLA", "AAPL", "AMD"])
    }

    func testEqualMovesKeepTheServerOrder() async throws {
        let model = try await popularModel([("AAPLx", 100), ("MSFTx", -100), ("NVDAx", 100)])

        XCTAssertEqual(model.topMovers.map(\.ticker), ["AAPL", "MSFT", "NVDA"])
    }

    func testRowsWithoutAMoveAreLeftOutOfTheRail() async throws {
        let model = try await popularModel([("AAPLx", nil), ("NVDAx", 418), ("MSFTx", -62)])

        XCTAssertEqual(model.topMovers.map(\.ticker), ["NVDA", "MSFT"])
    }

    func testFewerThanTwoMovesHidesTheRail() async throws {
        let model = try await popularModel([("AAPLx", nil), ("NVDAx", 418), ("MSFTx", nil)])

        XCTAssertTrue(model.topMovers.isEmpty)
    }

    func testPopularNowIsTheFirstThreeRowsInServerOrder() async throws {
        let model = try await popularModel([("AAPLx", 1), ("NVDAx", 418), ("MSFTx", 62), ("AMDx", -80)])

        XCTAssertEqual(model.popularNow.map(\.ticker), ["AAPL", "NVDA", "MSFT"])
    }

    func testPullToRefreshReloadsTheBrowsedListThePopularPageAndTheWatchlist() async throws {
        let transport = PrefixTransport([
            ("/v1/assets?filter=all", try Self.page(["TSLAx": 5])),
            ("/v1/assets?filter=popular", try Self.page(["NVDAx": 418])),
            ("/v1/assets/NVDAx", try Self.detailBody("NVDAx")),
        ])
        let model = makeModel(transport, symbols: ["NVDAx"])

        await model.loadMarkets()

        let targets = transport.sent
        XCTAssertEqual(targets.count, 3)
        XCTAssertEqual(targets.filter { $0.hasPrefix("/v1/assets?filter=all") }.count, 1)
        XCTAssertEqual(targets.filter { $0.hasPrefix("/v1/assets?filter=popular") }.count, 1)
        XCTAssertEqual(targets.filter { $0.hasPrefix("/v1/assets/NVDAx") }.count, 1)
        XCTAssertEqual(model.all.rows.map(\.ticker), ["TSLA"])
        XCTAssertEqual(model.popular.rows.map(\.ticker), ["NVDA"])
        XCTAssertEqual(model.quotes?.rows.map(\.id), ["NVDAx"])
    }

    func testBrowsingPopularAsksForThePopularPageOncePerRefresh() async throws {
        let transport = StubTransport(.json(.ok, try Self.page(["NVDAx": 418])))
        let model = makeModel(transport, symbols: [])
        await model.show(.popular)
        let before = await targets(transport).count

        await model.loadMarkets()

        let after = await targets(transport)
        XCTAssertEqual(before, 1)
        XCTAssertEqual(after.count, 2)
        XCTAssertTrue(after.allSatisfy { $0.contains("filter=popular") }, after.description)
    }

    func testAFailedRefreshWithRowsOnScreenMakesTheTabStaleFromTheLastGoodLoad() async throws {
        let moment = TestMoment()
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.page(["TSLAx": 5])), .failure(URLError(.cannotConnectToHost)),
        ])
        let model = makeModel(transport, symbols: [], moment: moment)
        await model.load()
        XCTAssertFalse(model.isStale)
        XCTAssertNil(model.staleMinutes)
        moment.advance(minutes: 14)

        await model.load()

        XCTAssertTrue(model.isStale)
        XCTAssertEqual(model.staleMinutes, 14)
        XCTAssertEqual(model.rows.map(\.ticker), ["TSLA"])
    }

    func testStaleMinutesKeepCountingFromTheLastGoodLoad() async throws {
        let moment = TestMoment()
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.page(["TSLAx": 5])), .failure(URLError(.cannotConnectToHost)),
        ])
        let model = makeModel(transport, symbols: [], moment: moment)
        await model.load()
        moment.advance(minutes: 3)
        await model.load()

        moment.advance(minutes: 4)

        XCTAssertEqual(model.staleMinutes, 7)
    }

    func testASuccessfulRefreshClearsTheStaleNotice() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.page(["TSLAx": 5])), .failure(URLError(.cannotConnectToHost)),
            .json(.ok, try Self.page(["TSLAx": 6])),
        ])
        let model = makeModel(transport, symbols: [])
        await model.load()
        await model.load()
        XCTAssertTrue(model.isStale)

        await model.load()

        XCTAssertFalse(model.isStale)
        XCTAssertNil(model.staleMinutes)
    }

    func testAFirstLoadThatFailsIsAnErrorNotAStaleTab() async throws {
        let model = makeModel(StubTransport(.failure(URLError(.cannotConnectToHost))), symbols: [])

        await model.load()

        XCTAssertFalse(model.isStale)
        XCTAssertTrue(model.rows.isEmpty)
    }

    func testAFailedPageTwoIsNotStale() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.page(["TSLAx": 5], cursor: "next")), .failure(URLError(.cannotConnectToHost)),
        ])
        let model = makeModel(transport, symbols: [])
        await model.load()

        await model.loadMore()

        XCTAssertFalse(model.isStale)
        XCTAssertEqual(model.rows.map(\.ticker), ["TSLA"])
    }

    func testSwitchingTheFilterClearsAStaleNoticeFromTheOtherList() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.page(["TSLAx": 5])), .failure(URLError(.cannotConnectToHost)),
            .json(.ok, try Self.page(["NVDAx": 418])),
        ])
        let model = makeModel(transport, symbols: [])
        await model.load()
        await model.load()
        XCTAssertTrue(model.isStale)

        await model.show(.popular)

        XCTAssertFalse(model.isStale)
    }

    func testAPriceHintThatFailsToRefreshMakesTheTabStaleWithoutAToast() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.page(["TSLAx": 5])), .failure(URLError(.cannotConnectToHost)),
            .failure(URLError(.cannotConnectToHost)),
        ])
        let hints = FakeHintStream()
        let model = makeModel(transport, symbols: [], hints: hints)
        await model.load()
        let observing = Task { await model.observe() }
        addTeardownBlock { observing.cancel() }
        _ = await waitUntil { await hints.subscriberCount > 0 }

        await hints.send(.changed(.global, what: "prices_updated", id: "1"))

        let stale = await waitUntil { model.isStale }
        XCTAssertTrue(stale)
        XCTAssertEqual(model.rows.map(\.ticker), ["TSLA"])
        XCTAssertEqual(model.failureTick, 0)
    }

    func testTheFilterOptionsMapToTheWireValues() async {
        XCTAssertEqual(StocksTabModel.Browse.allCases.map(\.identifier), ["all", "popular", "pre_ipo"])
    }

    func testOnlyTheDefaultSortIsAvailable() async {
        XCTAssertEqual(
            StocksTabModel.Sort.allCases.map(\.title),
            ["Default", "Volume", "Trade count", "Name A–Z", "Price change"])
        XCTAssertEqual(StocksTabModel.Sort.allCases.filter(\.isAvailable), [.standard])
        XCTAssertEqual(StocksTabModel.Sort.unavailableNote, "More sorts open soon")
    }

    func testTheStaleCopyReadsTheWayTheScreensDo() async {
        XCTAssertEqual(MarketStaleness.pricesDelayed(minutes: 14), "Prices delayed · 14 minutes ago.")
        XCTAssertEqual(MarketStaleness.pricesDelayed(minutes: 1), "Prices delayed · 1 minute ago.")
        XCTAssertEqual(MarketStaleness.pricesDelayed(minutes: 0), "Prices delayed · just now.")
        XCTAssertEqual(MarketStaleness.tokenPriceDelayed(minutes: 14), "Token price delayed · 14 minutes ago.")
    }

    private func popularModel(_ moves: [(String, Int?)]) async throws -> StocksTabModel {
        let transport = StubTransport(.json(.ok, try Self.page(moves)))
        let model = makeModel(transport, symbols: [])
        await model.loadPopular()
        return model
    }

    private func waitUntil(_ predicate: @escaping () async -> Bool) async -> Bool {
        for _ in 0..<500 {
            if await predicate() { return true }
            await Task.yield()
        }
        return await predicate()
    }

    private func makeModel(
        _ transport: any ClientTransport, symbols: [String], moment: TestMoment = TestMoment(),
        hints: FakeHintStream = FakeHintStream()
    ) -> StocksTabModel {
        let watchlist = WatchlistModel(userID: "u1", store: InMemoryWatchlistStore(["u1": symbols]))
        return StocksTabModel(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            hints: hints, clock: TestClock(), watchlist: watchlist, now: moment.now)
    }

    private func targets(_ transport: StubTransport) async -> [String] {
        await transport.sent.map { $0.path ?? "" }
    }

    private static func page(_ moves: [String: Int], cursor: String? = nil) throws -> String {
        try page(moves.sorted { $0.key < $1.key }.map { ($0.key, Optional($0.value)) }, cursor: cursor)
    }

    private static func page(_ moves: [(String, Int?)], cursor: String? = nil) throws -> String {
        let assets = moves.map { symbol, change -> Components.Schemas.AssetSummary in
            var asset = Components.Schemas.AssetSummary.googl
            asset.symbol = symbol
            asset.displayName = "\(symbol.dropLast()) xStock"
            asset.changeBps = change.map(Int32.init)
            return asset
        }
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        let list = Components.Schemas.AssetList(assets: assets, nextCursor: cursor)
        return String(decoding: try encoder.encode(list), as: UTF8.self)
    }

    private static func detailBody(_ symbol: String) throws -> String {
        var detail = Components.Schemas.AssetDetail.googl
        detail.symbol = symbol
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return String(decoding: try encoder.encode(detail), as: UTF8.self)
    }
}

private final class PrefixTransport: ClientTransport, Sendable {
    private let bodies: [(prefix: String, body: String)]
    private let paths = Mutex<[String]>([])

    init(_ bodies: [(prefix: String, body: String)]) {
        self.bodies = bodies
    }

    var sent: [String] { paths.withLock { $0 } }

    func send(_ request: HTTPRequest, body _: HTTPBody?, baseURL _: URL, operationID _: String) async throws
        -> (HTTPResponse, HTTPBody?)
    {
        let path = request.path ?? ""
        paths.withLock { $0.append(path) }
        guard let match = bodies.first(where: { path.hasPrefix($0.prefix) }) else { throw URLError(.unsupportedURL) }
        var response = HTTPResponse(status: .ok)
        response.headerFields[.contentType] = "application/json"
        return (response, HTTPBody(Data(match.body.utf8)))
    }
}
