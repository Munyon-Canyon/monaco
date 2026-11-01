import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class WatchlistQuotesTests: XCTestCase {
    func testEveryWatchedSymbolIsQuotedInTheOrderItWasAdded() async throws {
        let symbols = ["NVDAx", "AAPLx", "MSFTx", "TSLAx"]
        let transport = StubTransport(routes: [
            Self.batchPath(symbols): [
                try Self.batch([
                    ("TSLAx", "Tesla xStock", 262_500_000), ("NVDAx", "NVIDIA xStock", 220_000_000),
                    ("MSFTx", "Microsoft xStock", 350_000_000), ("AAPLx", "Apple xStock", 240_000_000),
                ])
            ]
        ])
        let (quotes, _) = makeQuotes(transport, symbols: symbols)

        await quotes.load()

        XCTAssertEqual(quotes.rows.map(\.id), symbols)
        guard case .quoted(let first) = quotes.rows[0] else { return XCTFail("NVDAx should be quoted") }
        XCTAssertEqual(first.name, "NVIDIA")
        XCTAssertEqual(first.priceText, "$220.00")
        let paths = await transport.sent.map { $0.path ?? "" }
        XCTAssertEqual(paths, [Self.batchPath(symbols)])
    }

    func testAWatchlistOfTwentyStocksCostsOneRequest() async throws {
        let symbols = (1...20).map { "S\($0)x" }
        let transport = StubTransport(routes: [
            Self.batchPath(symbols): [try Self.batch(symbols.map { ($0, "\($0) xStock", 100_000_000) })]
        ])
        let (quotes, _) = makeQuotes(transport, symbols: symbols)

        await quotes.load()

        XCTAssertEqual(quotes.rows.count, 20)
        let sent = await transport.sent
        XCTAssertEqual(sent.count, 1)
    }

    func testRowsAreLoadingUntilTheFirstAnswerArrives() async {
        let (quotes, _) = makeQuotes(StubTransport(routes: [:]), symbols: ["NVDAx", "AAPLx"])

        XCTAssertEqual(quotes.rows, [.loading(symbol: "NVDAx"), .loading(symbol: "AAPLx")])
    }

    func testAnEmptyWatchlistAsksForNothing() async {
        let transport = StubTransport(routes: [:])
        let (quotes, _) = makeQuotes(transport, symbols: [])

        await quotes.load()

        XCTAssertTrue(quotes.rows.isEmpty)
        let sent = await transport.sent
        XCTAssertTrue(sent.isEmpty)
    }

    func testABatchThatFailsLeavesEveryRowWithNothingToQuote() async throws {
        let transport = StubTransport(routes: [
            Self.batchPath(["NVDAx", "AAPLx"]): [.failure(URLError(.cannotConnectToHost))]
        ])
        let (quotes, watchlist) = makeQuotes(transport, symbols: ["NVDAx", "AAPLx"])

        await quotes.load()

        XCTAssertEqual(quotes.rows, [.unavailable(symbol: "NVDAx"), .unavailable(symbol: "AAPLx")])
        XCTAssertEqual(watchlist.symbols, ["NVDAx", "AAPLx"])
    }

    func testAFailedRefreshKeepsTheLastQuote() async throws {
        let transport = StubTransport(routes: [
            Self.batchPath(["NVDAx"]): [
                try Self.batch([("NVDAx", "NVIDIA xStock", 220_000_000)]), .failure(URLError(.cannotConnectToHost)),
            ]
        ])
        let (quotes, _) = makeQuotes(transport, symbols: ["NVDAx"])
        await quotes.load()

        await quotes.load()

        guard case .quoted(let asset) = quotes.rows[0] else { return XCTFail("the old quote should stay") }
        XCTAssertEqual(asset.priceText, "$220.00")
    }

    func testASymbolTheServerLeavesOutIsRemovedFromTheWatchlistSilently() async throws {
        let transport = StubTransport(routes: [
            Self.batchPath(["OLDx", "NVDAx"]): [try Self.batch([("NVDAx", "NVIDIA xStock", 220_000_000)])]
        ])
        let (quotes, watchlist) = makeQuotes(transport, symbols: ["OLDx", "NVDAx"])

        await quotes.load()

        XCTAssertEqual(watchlist.symbols, ["NVDAx"])
        XCTAssertEqual(quotes.rows.map(\.id), ["NVDAx"])
    }

    func testLoadingMissingQuotesAsksOnlyForSymbolsWithNoQuoteYet() async throws {
        let transport = StubTransport(routes: [
            Self.batchPath(["NVDAx"]): [try Self.batch([("NVDAx", "NVIDIA xStock", 220_000_000)])],
            Self.batchPath(["AAPLx"]): [try Self.batch([("AAPLx", "Apple xStock", 240_000_000)])],
        ])
        let (quotes, watchlist) = makeQuotes(transport, symbols: ["NVDAx"])
        await quotes.load()

        watchlist.toggle("AAPLx")
        await quotes.loadMissing()

        XCTAssertEqual(quotes.rows.map(\.id), ["NVDAx", "AAPLx"])
        let paths = await transport.sent.map { $0.path ?? "" }
        XCTAssertEqual(paths, [Self.batchPath(["NVDAx"]), Self.batchPath(["AAPLx"])])
    }

    func testAStarThatComesOffDropsItsQuote() async throws {
        let transport = StubTransport(routes: [
            Self.batchPath(["NVDAx", "AAPLx"]): [
                try Self.batch([("NVDAx", "NVIDIA xStock", 220_000_000), ("AAPLx", "Apple xStock", 240_000_000)])
            ]
        ])
        let (quotes, watchlist) = makeQuotes(transport, symbols: ["NVDAx", "AAPLx"])
        await quotes.load()

        watchlist.remove("NVDAx")

        XCTAssertEqual(quotes.rows.map(\.id), ["AAPLx"])
    }

    func testAnOldServerWithoutTheBatchRouteIsReadOneSymbolAtATime() async throws {
        let transport = StubTransport(routes: [
            Self.batchPath(["NVDAx", "AAPLx"]): [try Self.problem(status: 404, code: .assetNotFound)],
            "/v1/assets/NVDAx": [try Self.quote("NVDAx", "NVIDIA xStock", 220_000_000)],
            "/v1/assets/AAPLx": [try Self.quote("AAPLx", "Apple xStock", 240_000_000)],
        ])
        let (quotes, _) = makeQuotes(transport, symbols: ["NVDAx", "AAPLx"])

        await quotes.load()

        XCTAssertEqual(quotes.rows.map(\.id), ["NVDAx", "AAPLx"])
        guard case .quoted(let second) = quotes.rows[1] else { return XCTFail("AAPLx should be quoted") }
        XCTAssertEqual(second.priceText, "$240.00")
        let paths = await transport.sent.map { $0.path ?? "" }
        XCTAssertEqual(
            Set(paths), [Self.batchPath(["NVDAx", "AAPLx"]), "/v1/assets/NVDAx", "/v1/assets/AAPLx"])
        XCTAssertEqual(paths.count, 3)
    }

    func testAnOldServerStillDropsASymbolItDoesNotKnow() async throws {
        let transport = StubTransport(routes: [
            Self.batchPath(["OLDx", "NVDAx"]): [try Self.problem(status: 404, code: .assetNotFound)],
            "/v1/assets/NVDAx": [try Self.quote("NVDAx", "NVIDIA xStock", 220_000_000)],
            "/v1/assets/OLDx": [try Self.problem(status: 404, code: .assetNotFound)],
        ])
        let (quotes, watchlist) = makeQuotes(transport, symbols: ["OLDx", "NVDAx"])

        await quotes.load()

        XCTAssertEqual(watchlist.symbols, ["NVDAx"])
        XCTAssertEqual(quotes.rows.map(\.id), ["NVDAx"])
    }

    private func makeQuotes(_ transport: StubTransport, symbols: [String]) -> (WatchlistQuotes, WatchlistModel) {
        let watchlist = WatchlistModel(userID: "u1", store: InMemoryWatchlistStore(["u1": symbols]))
        let api = APIClient(
            serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        return (WatchlistQuotes(api: api, watchlist: watchlist), watchlist)
    }

    private static func batchPath(_ symbols: [String]) -> String {
        "/v1/assets/quotes?symbols=" + symbols.joined(separator: "%2C")
    }

    private static func problem(status: Int, code: Components.Schemas.ErrorCode) throws -> StubTransport.Reply {
        try .problem(
            Components.Schemas.Problem(
                _type: .about_colon_blank, title: "Not found", status: status, code: code,
                message: "No such asset.", traceId: "4bf92f3577b34da6a3ce929d0e0e4736", retryable: false))
    }

    private static func batch(_ quotes: [(String, String, Int64)]) throws -> StubTransport.Reply {
        var assets: [Components.Schemas.AssetSummary] = []
        for (symbol, name, micros) in quotes {
            var summary = Components.Schemas.AssetSummary.tsla
            summary.symbol = symbol
            summary.displayName = name
            summary.priceMicros = micros
            assets.append(summary)
        }
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        let body = try encoder.encode(Components.Schemas.AssetQuotes(assets: assets))
        return .json(.ok, String(decoding: body, as: UTF8.self))
    }

    private static func quote(_ symbol: String, _ name: String, _ micros: Int64) throws -> StubTransport.Reply {
        var detail = Components.Schemas.AssetDetail.googl
        detail.symbol = symbol
        detail.displayName = name
        detail.priceMicros = micros
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return .json(.ok, String(decoding: try encoder.encode(detail), as: UTF8.self))
    }
}
