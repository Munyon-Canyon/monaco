import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class CabalHoldingPerformanceTests: XCTestCase {
    private let cabalID = "01890a5d-ac96-774b-bcce-b302099a8060"
    private let symbol = "NVDAx"
    private let now = Date(timeIntervalSince1970: 1_790_996_400)

    func testThePerformanceReadFillsAverageSellRealizedAndTheCurves() async throws {
        let model = make(
            performance: performanceReply(avgSell: "230000000"))

        await model.load()

        let holding = try XCTUnwrap(model.holding)
        XCTAssertEqual(holding.figures.averageSellMicros, 230_000_000)
        XCTAssertEqual(holding.figures.realizedMicros, 117_860_000)
        XCTAssertEqual(holding.stats.map(\.value)[2], "$230.00")
        XCTAssertEqual(holding.stats.map(\.value)[4], "+$117.86")
        let curve = try XCTUnwrap(holding.curve(for: .oneWeek))
        XCTAssertEqual(curve.samples.map(\.pnl), [-5_000_000, 117_860_000])
        XCTAssertEqual(curve.samples.map(\.at), [now.addingTimeInterval(-3_600), now])
        XCTAssertNil(holding.curve(for: .oneMonth))
        XCTAssertNil(holding.curve(for: .all))
        XCTAssertEqual(holding.returnLabel(for: .oneWeek), "Position P&L · 1W")
    }

    func testAPositionWithNothingSoldSaysNoSalesYet() async throws {
        let model = make(
            performance: performanceReply(avgSell: nil, realized: 0))

        await model.load()

        let holding = try XCTUnwrap(model.holding)
        XCTAssertEqual(holding.stats.map(\.value)[2], CabalHoldingCopy.noSales)
        XCTAssertEqual(holding.stats.map(\.value)[4], CabalHoldingCopy.noSales)
        XCTAssertEqual(holding.stats[2].tone, .neutral)
        XCTAssertNotEqual(holding.stats[4].tone, .missing)
    }

    func testAFailedPerformanceReadKeepsTheDashesAndTheRestOfThePage() async throws {
        let model = make(
            performance: .failure(URLError(.notConnectedToInternet)))

        await model.load()

        let holding = try XCTUnwrap(model.holding)
        XCTAssertEqual(holding.stats.map(\.value)[2], "—")
        XCTAssertEqual(holding.stats.map(\.value)[4], "—")
        XCTAssertFalse(holding.hasSeries)
        XCTAssertEqual(holding.unitsText, "30.0000")
    }

    private func performanceReply(avgSell: String?, realized: Int64 = 117_860_000) -> StubTransport.Reply {
        let formatter = ISO8601DateFormatter()
        let earlier = formatter.string(from: now.addingTimeInterval(-3_600))
        let latest = formatter.string(from: now)
        let sell = avgSell.map { "\"\($0)\"" } ?? "null"
        let week =
            avgSell == nil
            ? "[]"
            : #"[{"at":"\#(earlier)","pnl_micros":-5000000},{"at":"\#(latest)","pnl_micros":117860000}]"#
        return .json(
            .ok,
            """
            {"avg_sell_micros":\(sell),"realized_pnl_micros":\(realized),\
            "series":{"1W":\(week),"1M":[],"3M":[],"1Y":[],"ALL":[]}}
            """)
    }

    private func make(performance: StubTransport.Reply) -> CabalHoldingModel {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        let encoded = (try? encoder.encode(Components.Schemas.Cabal.sample(role: nil))) ?? Data()
        let cabal = String(decoding: encoded, as: UTF8.self)
        let asOf = ISO8601DateFormatter().string(from: now)
        let pot = StubTransport.Reply.json(
            .ok,
            """
            {"cabal_id":"\(cabalID)","pot_value_micros":6600000000,"cash_micros":0,"cash_weight_bps":0,\
            "pnl_micros":407140000,"return_bps":null,"prices_as_of":"\(asOf)","holdings":[{"symbol":"\(symbol)",\
            "display_name":"NVIDIA","kind":"equity","units":"30.0000","token_amount":3000000000,\
            "price_micros":220000000,"value_micros":6600000000,"weight_bps":10000,"cost_basis_micros":6192860000,\
            "pnl_micros":407140000,"avg_entry_micros":null}],"me":null}
            """)
        let activity = StubTransport.Reply.json(.ok, #"{"items":[],"next_cursor":null}"#)
        let transport = StubTransport(
            routes: [
                "/v1/cabals/\(cabalID)/pot": [pot],
                "/v1/cabals/\(cabalID)": [.json(.ok, cabal), .json(.ok, cabal)],
                "/v1/cabals/\(cabalID)/holdings/\(symbol)/performance": [performance],
                "/v1/cabals/\(cabalID)/activity?limit=100": [activity],
            ])
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        return CabalHoldingModel(cabalID: cabalID, symbol: symbol, api: api, now: { [now] in now })
    }
}
