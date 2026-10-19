import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class CabalHoldingModelTests: XCTestCase {
    private typealias Activity = Components.Schemas.CabalActivity
    private let cabalID = "01890a5d-ac96-774b-bcce-b302099a8060"
    private let symbol = "NVDAx"
    private let now = Date(timeIntervalSince1970: 1_790_996_400)

    func testALoadedHoldingShowsTheCabalsPositionInTheSymbol() async throws {
        let (model, _) = make(pot: try potReply(holdingSymbol: symbol), activity: [try activityReply([])])

        await model.load()

        guard case .loaded(let holding) = model.outcome else { return XCTFail("got \(model.outcome)") }
        XCTAssertEqual(holding.valueText, "$6,600.00")
        XCTAssertEqual(holding.returnText(for: .oneMonth), "+$407.14")
        XCTAssertEqual(holding.unitsText, "30.0000")
        XCTAssertEqual(holding.cabalName, "QA pot")
        XCTAssertEqual(model.navigationTitle, "NVIDIA holding")
    }

    func testACabalThatDoesNotHoldTheSymbolIsEmptyWhenItHasNoTrades() async throws {
        let (model, _) = make(pot: try potReply(holdingSymbol: "AAPLx"), activity: [try activityReply([])])

        await model.load()

        guard case .empty(let holding) = model.outcome else { return XCTFail("got \(model.outcome)") }
        XCTAssertEqual(holding.valueText, "$0.00")
        XCTAssertEqual(holding.unitsText, "0")
        XCTAssertEqual(holding.stats.map(\.value), ["0", "—", "—", "—", "—", "—"])
        XCTAssertEqual(model.trades, .loaded([], hasMore: false))
    }

    func testASoldOutPositionWithTradesStillShowsTheHolding() async throws {
        let (model, _) = make(
            pot: try potReply(holdingSymbol: "AAPLx"), activity: [try activityReply([trade(1, .buy)])])

        await model.load()

        guard case .loaded = model.outcome else { return XCTFail("got \(model.outcome)") }
    }

    func testAFailedFirstLoadFails() async throws {
        let (model, _) = make(pot: .failure(URLError(.notConnectedToInternet)), activity: [try activityReply([])])

        await model.load()

        guard case .failed(.transport) = model.outcome else { return XCTFail("got \(model.outcome)") }
        XCTAssertEqual(model.navigationTitle, CabalHoldingCopy.fallbackTitle)
    }

    func testANonMemberGetsTheNotMemberOutcome() async throws {
        let (model, _) = make(pot: Self.notMember, activity: [try activityReply([])])

        await model.load()

        XCTAssertEqual(model.outcome, .notMember)
    }

    func testAFailedReloadKeepsTheHoldingAndSetsTheRefreshToast() async throws {
        let (model, _) = make(
            pots: [try potReply(holdingSymbol: symbol), .failure(URLError(.notConnectedToInternet))],
            activity: [try activityReply([]), try activityReply([])])

        await model.load()
        await model.load()

        guard case .loaded = model.outcome else { return XCTFail("got \(model.outcome)") }
        XCTAssertEqual(model.toast, CabalHoldingCopy.refreshFailed)
        model.dismissToast()
        XCTAssertNil(model.toast)
    }

    func testTradeHistoryKeepsOnlyConfirmedBuysAndSellsOfTheSymbol() async throws {
        let rows = [
            trade(1, .buy),
            trade(2, .sell),
            trade(3, .buy, status: .pending),
            trade(4, .buy, status: .failed),
            trade(5, .buy, symbol: "AAPLx"),
            Activity.sampleFund,
        ]
        let (model, _) = make(pot: try potReply(holdingSymbol: symbol), activity: [try activityReply(rows)])

        await model.load()

        guard case .loaded(let trades, let hasMore) = model.trades else { return XCTFail("got \(model.trades)") }
        XCTAssertEqual(trades.map(\.id), [trade(1, .buy).id, trade(2, .sell).id])
        XCTAssertEqual(trades.map(\.isBuy), [true, false])
        XCTAssertFalse(hasMore)
    }

    func testAFailedActivityReadFailsOnlyTheTradeHistory() async throws {
        let (model, _) = make(
            pot: try potReply(holdingSymbol: symbol), activity: [.failure(URLError(.notConnectedToInternet))])

        await model.load()

        guard case .loaded = model.outcome else { return XCTFail("got \(model.outcome)") }
        XCTAssertEqual(model.trades, .failed)
    }

    func testTheNextPageOfTradesIsOfferedAndAppended() async throws {
        let first = try activityReply([trade(1, .buy)], next: "c2")
        let second = try activityReply([trade(2, .sell)])
        let (model, transport) = make(
            pot: try potReply(holdingSymbol: symbol), activity: [first], more: [second], pagesPerLoad: 1)

        await model.load()
        guard case .loaded(let before, true) = model.trades else { return XCTFail("got \(model.trades)") }
        XCTAssertEqual(before.count, 1)

        await model.loadMoreTrades()

        guard case .loaded(let after, false) = model.trades else { return XCTFail("got \(model.trades)") }
        XCTAssertEqual(after.count, 2)
        let queries = await transport.sent.compactMap(\.path).filter { $0.contains("/activity") }
        XCTAssertEqual(queries.count, 2)
        XCTAssertTrue(queries[1].contains("cursor=c2"))
    }

    func testRetryingTheTradeHistoryReloadsIt() async throws {
        let (model, _) = make(
            pot: try potReply(holdingSymbol: symbol),
            activity: [.failure(URLError(.notConnectedToInternet)), try activityReply([trade(1, .buy)])])

        await model.load()
        XCTAssertEqual(model.trades, .failed)
        await model.retryTrades()

        guard case .loaded(let trades, _) = model.trades else { return XCTFail("got \(model.trades)") }
        XCTAssertEqual(trades.count, 1)
    }

    func testAStalePriceReadShowsTheMinutesSinceIt() async throws {
        let fresh = now.addingTimeInterval(-5 * 60)
        let old = now.addingTimeInterval(-14 * 60)

        let (freshModel, _) = make(
            pot: try potReply(holdingSymbol: symbol, pricesAsOf: fresh), activity: [try activityReply([])])
        let (oldModel, _) = make(
            pot: try potReply(holdingSymbol: symbol, pricesAsOf: old), activity: [try activityReply([])])
        await freshModel.load()
        await oldModel.load()

        XCTAssertNil(freshModel.holding?.staleMinutes)
        XCTAssertEqual(oldModel.holding?.staleMinutes, 14)
    }

    func testFiguresTheBackendLacksFallBackToDashesAndAllTime() async throws {
        let (model, _) = make(pot: try potReply(holdingSymbol: symbol), activity: [try activityReply([])])

        await model.load()

        let holding = try XCTUnwrap(model.holding)
        XCTAssertFalse(holding.hasSeries)
        XCTAssertTrue(holding.hasMissingFigures)
        XCTAssertEqual(
            holding.stats.map(\.value), ["30.0000", "—", "—", "$6,192.86", "—", "+$407.14"])
        XCTAssertEqual(holding.returnLabel(for: .oneMonth), "Position P&L · All time")
        XCTAssertEqual(holding.returnText(for: .oneMonth), "+$407.14")
    }

    func testSuppliedFiguresFillTheGapsAndSwitchTheReturnToThePeriod() async throws {
        let figures = CabalHoldingFixture.figures(loss: false, now: now)
        let (model, _) = make(
            pot: try potReply(holdingSymbol: symbol), activity: [try activityReply([])], figures: figures)

        await model.load()

        let holding = try XCTUnwrap(model.holding)
        XCTAssertFalse(holding.hasMissingFigures)
        XCTAssertEqual(holding.returnText(for: .oneMonth), "+$525.00")
        XCTAssertEqual(holding.returnLabel(for: .oneMonth), "Position P&L · 1M")
        XCTAssertEqual(holding.stats.map(\.value)[1...2], ["$206.43", "$230.00"])
    }

    private func make(
        pot: StubTransport.Reply, activity: [StubTransport.Reply], more: [StubTransport.Reply] = [],
        figures: CabalHolding.Figures = .none, pagesPerLoad: Int = CabalHoldingModel.pagesPerLoad
    ) -> (CabalHoldingModel, StubTransport) {
        make(pots: [pot], activity: activity, more: more, figures: figures, pagesPerLoad: pagesPerLoad)
    }

    private func make(
        pots: [StubTransport.Reply], activity: [StubTransport.Reply], more: [StubTransport.Reply] = [],
        figures: CabalHolding.Figures = .none, pagesPerLoad: Int = CabalHoldingModel.pagesPerLoad
    ) -> (CabalHoldingModel, StubTransport) {
        let cabal = Self.encodedCabal()
        let cabalReply = cabal.map { StubTransport.Reply.json(.ok, $0) } ?? .failure(URLError(.badServerResponse))
        let transport = StubTransport(
            routes: [
                "/v1/cabals/\(cabalID)/pot": pots,
                "/v1/cabals/\(cabalID)": Array(repeating: cabalReply, count: pots.count * 2),
                "/v1/cabals/\(cabalID)/activity?limit=100": activity,
                "/v1/cabals/\(cabalID)/activity?limit=100&cursor=c2": more,
            ])
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        let model = CabalHoldingModel(
            cabalID: cabalID, symbol: symbol, api: api, figures: figures, pagesPerLoad: pagesPerLoad,
            now: { [now] in now })
        return (model, transport)
    }

    private func potReply(holdingSymbol: String, pricesAsOf: Date? = nil) throws -> StubTransport.Reply {
        let asOf = ISO8601DateFormatter().string(from: pricesAsOf ?? now)
        return .json(
            .ok,
            """
            {"cabal_id":"\(cabalID)","pot_value_micros":6600000000,"cash_micros":0,"cash_weight_bps":0,\
            "pnl_micros":407140000,"return_bps":null,"prices_as_of":"\(asOf)","holdings":[{"symbol":"\
            \(holdingSymbol)","display_name":"NVIDIA","kind":"equity","units":"30.0000","token_amount":3000000000,\
            "price_micros":220000000,"value_micros":6600000000,"weight_bps":10000,"cost_basis_micros":6192860000,\
            "pnl_micros":407140000}],"me":null}
            """)
    }

    private func trade(
        _ number: Int, _ kind: Activity.KindPayload, status: Activity.StatusPayload = .confirmed,
        symbol: String = "NVDAx"
    ) -> Activity {
        .sample(
            number, kind: kind, status: status, asset: .init(symbol: symbol, name: "NVIDIA"), micros: 200_000_000,
            actor: nil)
    }

    private func activityReply(_ items: [Activity], next: String? = nil) throws -> StubTransport.Reply {
        .json(.ok, try Self.encode(Components.Schemas.CabalActivityPage(items: items, nextCursor: next)))
    }

    private static func encodedCabal() -> String? {
        try? encode(Components.Schemas.Cabal.sample(role: nil))
    }

    private static func encode(_ value: some Encodable) throws -> String {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return String(decoding: try encoder.encode(value), as: UTF8.self)
    }

    private static let notMember: StubTransport.Reply = {
        let body =
            #"{"type":"about:blank","title":"Error","status":403,"code":"not_cabal_member","#
            + #""message":"You are not a member of this cabal.","#
            + #""trace_id":"00000000000000000000000000000000","retryable":false}"#
        return .response(status: .forbidden, contentType: "application/problem+json", body: Data(body.utf8))
    }()
}
