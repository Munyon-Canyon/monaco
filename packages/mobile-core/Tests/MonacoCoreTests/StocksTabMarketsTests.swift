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
    func testPullToRefreshReloadsThePreIpoListTheWatchlistAndItsFacts() async throws {
        let transport = PrefixTransport([
            ("/v1/assets?filter=pre_ipo", try Self.page(["tSpaceX": 5])),
            ("/v1/assets?filter=movers", try Self.page(["TSLAx": -240])),
            ("/v1/assets?filter=popular", try Self.page(["AAPLx": 120])),
            ("/v1/me/market-facts", Self.factsBody),
        ])
        let model = makeModel(transport, symbols: ["NVDAx"])

        await model.loadMarkets()

        let targets = transport.sent
        XCTAssertEqual(targets.count, 5)
        XCTAssertEqual(targets.filter { $0.hasPrefix("/v1/assets?filter=popular") }.count, 1)
        XCTAssertEqual(model.stocksPreview.map(\.symbol), ["AAPLx"])
        XCTAssertEqual(targets.filter { $0.hasPrefix("/v1/assets?filter=movers") }.count, 1)
        XCTAssertEqual(targets.filter { $0.hasPrefix("/v1/assets?filter=pre_ipo") }.count, 1)
        XCTAssertEqual(targets.filter { $0.hasPrefix("/v1/assets/quotes?symbols=NVDAx") }.count, 1)
        XCTAssertEqual(targets.filter { $0.hasPrefix("/v1/me/market-facts") }.count, 1)
        XCTAssertEqual(model.preIpo.rows.map(\.symbol), ["tSpaceX"])
        XCTAssertEqual(model.rows.map(\.symbol), ["tSpaceX"])
        XCTAssertEqual(model.quotes?.rows.map(\.id), ["NVDAx"])
        XCTAssertEqual(model.facts["NVDAx"], .voteOpen(cabalName: "AC Capital", count: 1))
    }

    func testTheFactsRequestCarriesTheWatchlistSymbolsOncePerChange() async throws {
        let transport = PrefixTransport([("/v1/me/market-facts", Self.factsBody)])
        let watchlist = WatchlistModel(userID: "u1", store: InMemoryWatchlistStore(["u1": ["NVDAx", "AAPLx"]]))
        let model = makeModel(transport, watchlist: watchlist)

        await model.watchlistChanged()
        watchlist.toggle("MSFTx")
        await model.watchlistChanged()

        let asked = transport.sent.filter { $0.hasPrefix("/v1/me/market-facts") }
        XCTAssertEqual(asked.count, 2)
        XCTAssertTrue(asked[0].contains("symbols=NVDAx,AAPLx") || asked[0].contains("symbols=NVDAx%2CAAPLx"), asked[0])
        XCTAssertTrue(asked[1].contains("MSFTx"), asked[1])
    }

    func testAnEmptyWatchlistAsksForNoFacts() async throws {
        let transport = PrefixTransport([("/v1/me/market-facts", Self.factsBody)])
        let model = makeModel(transport, symbols: [])

        await model.watchlistChanged()

        XCTAssertTrue(transport.sent.isEmpty)
        XCTAssertTrue(model.facts.isEmpty)
    }

    func testAFailedFactsReadKeepsTheLastFactsAndRaisesNoToast() async throws {
        let transport = PrefixTransport([("/v1/me/market-facts", Self.factsBody)])
        let model = makeModel(transport, symbols: ["NVDAx"])
        await model.watchlistChanged()
        let failing = makeModel(StubTransport(.failure(URLError(.cannotConnectToHost))), symbols: ["NVDAx"])

        await failing.watchlistChanged()

        XCTAssertEqual(model.facts.count, 1)
        XCTAssertTrue(failing.facts.isEmpty)
        XCTAssertEqual(failing.failureTick, 0)
    }

    func testTheWatchlistPreviewLeavesPreIpoNamesToTheirOwnSectionAndStopsAtFive() async throws {
        let symbols = ["tSpaceX", "A1x", "A2x", "A3x", "A4x", "A5x", "A6x"]
        let transport = PrefixTransport([("/v1/me/market-facts", Self.factsBody)])
        let model = makeModel(transport, symbols: symbols)

        await model.watchlistChanged()

        XCTAssertEqual(model.watchlistRows.count, 7)
        XCTAssertEqual(model.watchlistCount, 7)
        XCTAssertEqual(model.watchlistPreview.count, 5)
        XCTAssertFalse(model.watchlistPreview.contains { $0.id == "tSpaceX" })
    }

    func testSeeAllOnTheAllViewSelectsTheChipWithoutReloadingThePreIpoList() async throws {
        let transport = PrefixTransport([
            ("/v1/assets?filter=pre_ipo", try Self.page(["tSpaceX": 5])),
            ("/v1/me/market-facts", Self.factsBody),
        ])
        let model = makeModel(transport, symbols: [])
        await model.loadMarkets()
        let before = transport.sent.count

        await model.show(.watchlist)
        await model.show(.preIpo)

        XCTAssertEqual(model.browse, .preIpo)
        XCTAssertEqual(transport.sent.count, before)
        XCTAssertEqual(model.rows.map(\.symbol), ["tSpaceX"])
    }

    func testAPreIpoRowReadsTheSecondaryEstimateNotThePriceOrAPercent() async throws {
        let transport = PrefixTransport([
            ("/v1/assets?filter=pre_ipo", try Self.preIpoPage()), ("/v1/me/market-facts", Self.factsBody),
        ])
        let model = makeModel(transport, symbols: [])

        await model.loadMarkets()

        let row = try XCTUnwrap(model.preIpoPreview.first)
        XCTAssertEqual(row.estimate, MarketEstimate(priceMicros: 185_000_000, asOf: "2026-10-20"))
        XCTAssertEqual(PreIpoRowCopy.estimateLine(asOf: "2026-10-20"), "Secondary estimate · Oct 20")
        XCTAssertFalse(PreIpoRowCopy.estimateLine(asOf: "2026-10-20").contains("%"))
        XCTAssertEqual(PreIpoRowCopy.eligibility(tradable: true), "Can buy")
        XCTAssertEqual(PreIpoRowCopy.eligibility(tradable: false), "Watch only")
        XCTAssertEqual(
            PreIpoRowCopy.footer(asOfDates: ["2026-10-20", "2026-10-09", "2026-10-15"]),
            "Prices are secondary-market estimates from Oct 9 to Oct 20.")
        XCTAssertEqual(
            PreIpoRowCopy.footer(asOfDates: ["2026-10-20", "2026-10-20"]),
            "Prices are secondary-market estimates from Oct 20.")
        XCTAssertNil(PreIpoRowCopy.footer(asOfDates: []))
        XCTAssertEqual(
            MarketsInfoCopy.preIpo,
            "Pre-IPO companies are private. Their price is a secondary estimate between funding rounds, "
                + "not a live quote. You can propose to buy the ones marked Can buy; "
                + "the ones marked Watch only can't be bought here yet.")
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

        await model.show(.preIpo)

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
        XCTAssertEqual(StocksTabModel.Browse.allCases.map(\.identifier), ["all", "watchlist", "pre_ipo", "held"])
    }

    func testTheStaleCopyReadsTheWayTheScreensDo() async {
        XCTAssertEqual(MarketStaleness.pricesDelayed(minutes: 14), "Prices delayed · 14 minutes ago.")
        XCTAssertEqual(MarketStaleness.pricesDelayed(minutes: 1), "Prices delayed · 1 minute ago.")
        XCTAssertEqual(MarketStaleness.pricesDelayed(minutes: 0), "Prices delayed · just now.")
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
        return makeModel(transport, watchlist: watchlist, moment: moment, hints: hints)
    }

    private func makeModel(
        _ transport: any ClientTransport, watchlist: WatchlistModel, moment: TestMoment = TestMoment(),
        hints: FakeHintStream = FakeHintStream()
    ) -> StocksTabModel {
        StocksTabModel(
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

    private static let factsBody =
        #"{"facts":[{"symbol":"NVDAx","fact":{"kind":"vote_open","cabal_name":"AC Capital","count":1}}]}"#

    private static func preIpoPage() throws -> String {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        let list = Components.Schemas.AssetList(assets: [.spaceX], nextCursor: nil)
        return String(decoding: try encoder.encode(list), as: UTF8.self)
    }
}

extension StocksTabMarketsTests {
    func testThePreIpoRoundLineSaysWhatTheCompanyIsValuedAtAndWhen() async throws {
        let today = Date(timeIntervalSince1970: 1_793_750_400)
        func line(_ valuation: String?, _ round: String?, _ estimate: String? = nil) -> String {
            PreIpoRowCopy.roundLine(
                valuationMicros: valuation, lastRoundOn: round, estimateAsOf: estimate, ticker: "SPACEX", today: today)
        }
        XCTAssertEqual(line("350000000000000000", "2026-06-01"), "Valued at $350B in June")
        XCTAssertEqual(line("350000000000000000", "2025-06-01"), "Valued at $350B in June 2025")
        XCTAssertEqual(line("350000000000000000", "2025-12-15"), "Valued at $350B in December")
        XCTAssertEqual(line("350000000000000000", "2025-11-30"), "Valued at $350B in November 2025")
        XCTAssertEqual(line("1600000000000000000", "2026-03-01"), "Valued at $1.6T in March")
        XCTAssertEqual(line("500000000000000", "2026-03-01"), "Valued at $500M in March")
        XCTAssertEqual(line("350000000000000000", nil), "Valued at $350B")
        XCTAssertEqual(line("350000000000000000", "soon"), "Valued at $350B")
        XCTAssertEqual(line(nil, "2026-06-01", "2026-10-20"), "Secondary estimate · Oct 20")
        XCTAssertEqual(line("", nil, "2026-10-20"), "Secondary estimate · Oct 20")
        XCTAssertEqual(line(nil, nil), "SPACEX")
    }

    func testMappingReadsTheValuationAndTheLastRoundFromTheListSummary() async throws {
        let withRound = MarketMapping.asset(.spaceX)
        XCTAssertEqual(withRound.valuationMicros, "350000000000000000")
        XCTAssertEqual(withRound.lastRoundOn, "2026-06-01")
        let without = MarketMapping.asset(.googl)
        XCTAssertNil(without.valuationMicros)
        XCTAssertNil(without.lastRoundOn)
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
        var response = HTTPResponse(status: .ok)
        response.headerFields[.contentType] = "application/json"
        if path.hasPrefix("/v1/assets/quotes?symbols=") {
            return (response, HTTPBody(Data(Self.quotes(path).utf8)))
        }
        guard let match = bodies.first(where: { path.hasPrefix($0.prefix) }) else { throw URLError(.unsupportedURL) }
        return (response, HTTPBody(Data(match.body.utf8)))
    }

    private static func quotes(_ path: String) -> String {
        let query = path.dropFirst("/v1/assets/quotes?symbols=".count)
        let symbols = (String(query).removingPercentEncoding ?? "").split(separator: ",").map(String.init)
        var assets: [Components.Schemas.AssetSummary] = []
        for symbol in symbols {
            var asset: Components.Schemas.AssetSummary = symbol.hasPrefix("t") ? .spaceX : .googl
            asset.symbol = symbol
            assets.append(asset)
        }
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        let body = (try? encoder.encode(Components.Schemas.AssetQuotes(assets: assets))) ?? Data()
        return String(decoding: body, as: UTF8.self)
    }
}

extension StocksTabMarketsTests {
    func testStarringOneStockQuotesOnlyThatStock() async throws {
        let transport = PrefixTransport([("/v1/me/market-facts", Self.factsBody)])
        let watchlist = WatchlistModel(userID: "u1", store: InMemoryWatchlistStore(["u1": ["NVDAx", "AAPLx"]]))
        let model = makeModel(transport, watchlist: watchlist)
        await model.watchlistChanged()
        let before = transport.sent.filter { $0.hasPrefix("/v1/assets/") }.count

        watchlist.toggle("MSFTx")
        await model.watchlistChanged()

        let quoted = transport.sent.filter { $0.hasPrefix("/v1/assets/") }
        XCTAssertEqual(before, 1)
        XCTAssertEqual(quoted.count, 2)
        XCTAssertTrue(quoted[1].hasPrefix("/v1/assets/quotes?symbols=MSFTx"), quoted[1])
    }
}
