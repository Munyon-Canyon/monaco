import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class WatchlistQuotesTests: XCTestCase {
    func testEveryWatchedSymbolIsQuotedInTheOrderItWasAdded() async throws {
        let transport = StubTransport(routes: [
            "/v1/assets/NVDAx": [try Self.quote("NVDAx", "NVIDIA xStock", 220_000_000)],
            "/v1/assets/AAPLx": [try Self.quote("AAPLx", "Apple xStock", 240_000_000)],
            "/v1/assets/MSFTx": [try Self.quote("MSFTx", "Microsoft xStock", 350_000_000)],
            "/v1/assets/TSLAx": [try Self.quote("TSLAx", "Tesla xStock", 262_500_000)],
        ])
        let (quotes, _) = makeQuotes(transport, symbols: ["NVDAx", "AAPLx", "MSFTx", "TSLAx"])

        await quotes.load()

        XCTAssertEqual(quotes.rows.map(\.id), ["NVDAx", "AAPLx", "MSFTx", "TSLAx"])
        guard case .quoted(let first) = quotes.rows[0] else { return XCTFail("NVDAx should be quoted") }
        XCTAssertEqual(first.name, "NVIDIA")
        XCTAssertEqual(first.priceText, "$220.00")
        let paths = await transport.sent.map { $0.path ?? "" }
        XCTAssertEqual(Set(paths), ["/v1/assets/NVDAx", "/v1/assets/AAPLx", "/v1/assets/MSFTx", "/v1/assets/TSLAx"])
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

    func testAQuoteThatFailsStillShowsItsRowWithNothingToQuote() async throws {
        let transport = StubTransport(routes: [
            "/v1/assets/NVDAx": [try Self.quote("NVDAx", "NVIDIA xStock", 220_000_000)],
            "/v1/assets/AAPLx": [.failure(URLError(.cannotConnectToHost))],
        ])
        let (quotes, watchlist) = makeQuotes(transport, symbols: ["NVDAx", "AAPLx"])

        await quotes.load()

        XCTAssertEqual(quotes.rows.map(\.id), ["NVDAx", "AAPLx"])
        XCTAssertEqual(quotes.rows[1], .unavailable(symbol: "AAPLx"))
        XCTAssertEqual(watchlist.symbols, ["NVDAx", "AAPLx"])
    }

    func testAFailedRefreshKeepsTheLastQuote() async throws {
        let transport = StubTransport(routes: [
            "/v1/assets/NVDAx": [
                try Self.quote("NVDAx", "NVIDIA xStock", 220_000_000), .failure(URLError(.cannotConnectToHost)),
            ]
        ])
        let (quotes, _) = makeQuotes(transport, symbols: ["NVDAx"])
        await quotes.load()

        await quotes.load()

        guard case .quoted(let asset) = quotes.rows[0] else { return XCTFail("the old quote should stay") }
        XCTAssertEqual(asset.priceText, "$220.00")
    }

    func testASymbolTheServerDoesNotKnowIsRemovedFromTheWatchlistSilently() async throws {
        let gone = Components.Schemas.Problem(
            _type: .about_colon_blank, title: "Not found", status: 404, code: .assetNotFound,
            message: "No such asset.", traceId: "4bf92f3577b34da6a3ce929d0e0e4736", retryable: false)
        let transport = StubTransport(routes: [
            "/v1/assets/NVDAx": [try Self.quote("NVDAx", "NVIDIA xStock", 220_000_000)],
            "/v1/assets/OLDx": [try .problem(gone)],
        ])
        let (quotes, watchlist) = makeQuotes(transport, symbols: ["OLDx", "NVDAx"])

        await quotes.load()

        XCTAssertEqual(watchlist.symbols, ["NVDAx"])
        XCTAssertEqual(quotes.rows.map(\.id), ["NVDAx"])
    }

    func testAStarThatComesOffDropsItsQuote() async throws {
        let transport = StubTransport(routes: [
            "/v1/assets/NVDAx": [try Self.quote("NVDAx", "NVIDIA xStock", 220_000_000)],
            "/v1/assets/AAPLx": [try Self.quote("AAPLx", "Apple xStock", 240_000_000)],
        ])
        let (quotes, watchlist) = makeQuotes(transport, symbols: ["NVDAx", "AAPLx"])
        await quotes.load()

        watchlist.remove("NVDAx")

        XCTAssertEqual(quotes.rows.map(\.id), ["AAPLx"])
    }

    private func makeQuotes(_ transport: StubTransport, symbols: [String]) -> (WatchlistQuotes, WatchlistModel) {
        let watchlist = WatchlistModel(userID: "u1", store: InMemoryWatchlistStore(["u1": symbols]))
        let api = APIClient(
            serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        return (WatchlistQuotes(api: api, watchlist: watchlist), watchlist)
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
