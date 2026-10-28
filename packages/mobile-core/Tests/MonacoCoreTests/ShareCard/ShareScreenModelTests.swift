import Foundation
import HTTPTypes
import MonacoAPI
import MonacoTestSupport
import OpenAPIRuntime
import XCTest

@testable import MonacoCore

@MainActor
final class ShareScreenModelTests: XCTestCase {
    private let now = Date(timeIntervalSince1970: 1_790_000_000)
    private let subject = ShareSubject.position(cabalID: "cabal-1", symbol: "NVDAx")

    func testThePositionLoadsFromAllFiveReads() async throws {
        let server = ShareServer(tradePages: [3])
        let model = make(server)

        await model.load()

        let spec = try XCTUnwrap(model.spec)
        XCTAssertEqual(spec.header.title, Components.Schemas.Cabal.sample(role: "member").name)
        XCTAssertEqual(spec.header.subtitle, "Holding · 3 calls")
        XCTAssertEqual(spec.tickerLine.symbol, "NVDA")
        XCTAssertEqual(spec.figure.main, "+$1,027.90")
        let paths = await server.paths
        XCTAssertEqual(
            Set(paths),
            [
                "/v1/cabals/cabal-1/pot", "/v1/cabals/cabal-1", "/v1/cabals/cabal-1/activity",
                "/v1/assets/NVDAx/chart", "/v1/me/referral-code",
            ])
    }

    func testTheChartRangeCoversTheFirstBuy() async throws {
        let server = ShareServer(tradePages: [1])
        let model = make(server)

        await model.load()

        let queries = await server.chartQueries
        XCTAssertEqual(queries.count, 1)
        XCTAssertTrue(queries[0].contains("range=3M"), queries[0])
    }

    func testA250TradePositionLoadsEveryPageAndCountsAll() async throws {
        let server = ShareServer(tradePages: [100, 100, 50])
        let model = make(server)

        await model.load()

        let spec = try XCTUnwrap(model.spec)
        XCTAssertEqual(spec.header.subtitle, "Holding · 250 calls")
        XCTAssertEqual(spec.chart.markers.count, 250)
        let pages = await server.activityCursors
        XCTAssertEqual(pages, [nil, "page-1", "page-2"])
    }

    func testThePageStopTruncatesAndSaysSo() async throws {
        let server = ShareServer(tradePages: Array(repeating: 1, count: 5), endless: true)
        let model = make(server, maxPages: 3)

        await model.load()

        let spec = try XCTUnwrap(model.spec)
        XCTAssertEqual(spec.header.subtitle, "Holding · 3+ calls")
        let pages = await server.activityCursors
        XCTAssertEqual(pages.count, 3)
    }

    func testAnEyeFlipRebuildsTheSpecWithoutAnotherRead() async throws {
        let server = ShareServer(tradePages: [3])
        let model = make(server)
        await model.load()
        let before = await server.paths.count

        model.eyes.pnlDollars = false
        XCTAssertEqual(model.spec?.figure.main, "+14.0%")
        model.eyes.buySell = false
        XCTAssertEqual(model.spec?.chart.markers.count, 0)
        model.eyes.code = false
        XCTAssertNil(model.spec?.code)

        let after = await server.paths.count
        XCTAssertEqual(after, before)
    }

    func testAFailedReadIsTheFailedState() async {
        let server = ShareServer(tradePages: [3], failing: "/v1/cabals/cabal-1/pot")
        let model = make(server)

        await model.load()

        guard case .failed = model.state else { return XCTFail("got \(model.state)") }
        XCTAssertNil(model.spec)
        XCTAssertNil(model.shareURL)
    }

    func testAMissingPositionIsTheFailedState() async {
        let model = make(ShareServer(tradePages: [3]), subject: .position(cabalID: "cabal-1", symbol: "TSLAx"))

        await model.load()

        guard case .failed = model.state else { return XCTFail("got \(model.state)") }
    }

    func testTheShareURLCarriesTheCabalAndTheHandle() async throws {
        let model = make(ShareServer(tradePages: [1]))

        await model.load()

        let url = try XCTUnwrap(model.shareURL)
        XCTAssertEqual(url.path, "/r/kaicenat")
        XCTAssertEqual(URLComponents(url: url, resolvingAgainstBaseURL: false)?.queryItems?.first?.name, "c")
        XCTAssertEqual(URLComponents(url: url, resolvingAgainstBaseURL: false)?.queryItems?.first?.value, "cabal-1")
        XCTAssertEqual(model.spec?.code, "KAICENAT")
    }

    func testALockedHandleFallsBackToTheRandomCode() async throws {
        let model = make(ShareServer(tradePages: [1], locked: true))

        await model.load()

        XCTAssertEqual(model.spec?.code, "K7M4QX2P")
        XCTAssertEqual(model.shareURL?.path, "/r/k7m4qx2p")
    }

    private func make(_ server: ShareServer, subject: ShareSubject? = nil, maxPages: Int = 20) -> ShareScreenModel {
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: server)
        let now = now
        return ShareScreenModel(
            subject: subject ?? self.subject, api: api, now: { now }, timeZone: TimeZone(identifier: "UTC") ?? .gmt,
            maxPages: maxPages)
    }
}

private actor ShareServer: ClientTransport {
    private let tradePages: [Int]
    private let endless: Bool
    private let failing: String?
    private let locked: Bool
    private(set) var paths: [String] = []
    private(set) var chartQueries: [String] = []
    private(set) var activityCursors: [String?] = []

    init(tradePages: [Int], endless: Bool = false, failing: String? = nil, locked: Bool = false) {
        self.tradePages = tradePages
        self.endless = endless
        self.failing = failing
        self.locked = locked
    }

    func send(_ request: HTTPRequest, body _: HTTPBody?, baseURL _: URL, operationID _: String) async throws
        -> (HTTPResponse, HTTPBody?)
    {
        let components = URLComponents(string: request.path ?? "")
        let path = components?.path ?? ""
        paths.append(path)
        if path == failing { return reply(.internalServerError, "{}") }
        switch path {
        case "/v1/cabals/cabal-1/pot": return reply(.ok, Self.pot)
        case "/v1/cabals/cabal-1": return reply(.ok, try encode(Components.Schemas.Cabal.sample(role: "member")))
        case "/v1/me/referral-code":
            return reply(
                .ok,
                try encode(locked ? Components.Schemas.MyReferralCode.sampleLocked : .sample))
        case "/v1/assets/NVDAx/chart":
            chartQueries.append(components?.query ?? "")
            return reply(.ok, try encode(Components.Schemas.AssetChart.oneDay))
        case "/v1/cabals/cabal-1/activity":
            let cursor = components?.queryItems?.first { $0.name == "cursor" }?.value
            activityCursors.append(cursor)
            return reply(.ok, activityPage(cursor: cursor))
        default: return reply(.notFound, "{}")
        }
    }

    private func activityPage(cursor: String?) -> String {
        let index = cursor.flatMap { Int($0.dropFirst("page-".count)) } ?? 0
        let count = endless ? 1 : tradePages[index]
        let more = endless || index + 1 < tradePages.count
        let rows = (0..<count).map { row in
            let when = Date(timeIntervalSince1970: 1_780_000_000 - Double(index * 1_000 + row) * 3_600)
            return """
                {"id":"a-\(index)-\(row)","kind":"\(row % 2 == 0 ? "buy" : "sell")","status":"confirmed",\
                "asset":{"symbol":"NVDAx","name":"NVIDIA"},"usdc_micros":190000000,"units":100000000,\
                "price_micros":190000000,"actor":null,"tx_signature":null,\
                "occurred_at":"\(SharedFormatters.iso8601WholeSeconds.string(from: when))"}
                """
        }
        let next = more ? "\"page-\(index + 1)\"" : "null"
        return "{\"items\":[\(rows.joined(separator: ","))],\"next_cursor\":\(next)}"
    }

    private func reply(_ status: HTTPResponse.Status, _ body: String) -> (HTTPResponse, HTTPBody?) {
        var response = HTTPResponse(status: status)
        response.headerFields[.contentType] = "application/json"
        return (response, HTTPBody(body))
    }

    private func encode<T: Encodable>(_ value: T) throws -> String {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return String(decoding: try encoder.encode(value), as: UTF8.self)
    }

    private static let pot = """
        {"cabal_id":"cabal-1","pot_value_micros":8360000000,"cash_micros":0,"cash_weight_bps":0,\
        "pnl_micros":1027900000,"return_bps":1402,"prices_as_of":"2026-10-22T15:00:00Z","holdings":[\
        {"symbol":"NVDAx","display_name":"NVIDIA","kind":"equity","units":"38.0000","token_amount":3800000000,\
        "price_micros":220000000,"value_micros":8360000000,"weight_bps":10000,"cost_basis_micros":7332100000,\
        "pnl_micros":1027900000,"avg_entry_micros":"192950000","first_bought_at":"2026-08-20T15:00:00Z"}],\
        "me":{"share_units":100000000,"value_micros":8360000000,"slice_bps":10000,\
        "net_contributed_micros":7332100000,"pnl_micros":1027900000,"first_funded_at":"2026-08-19T15:00:00Z"}}
        """
}
