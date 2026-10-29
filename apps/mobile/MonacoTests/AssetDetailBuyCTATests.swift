import Foundation
import HTTPTypes
import MonacoAPI
import MonacoCore
import Observation
import OpenAPIRuntime
import Testing

@testable import Monaco

@MainActor
struct AssetDetailBuyCTATests {
    @Test func aPausedStockCannotBeBought() {
        #expect(!AssetDetailBuyCTA.canBuy(tradable: true, quotable: false))
        #expect(AssetDetailBuyCTA.canBuy(tradable: true, quotable: true))
    }

    @Test func theButtonsSayProposeBuyAndProposeSellAndTheBuyTurnsToUnavailableWhenTheStockCannotTrade() {
        #expect(AssetDetailBuyCTA.title(tradable: true) == "Propose buy")
        #expect(AssetDetailBuyCTA.sellTitle == "Propose sell")
        #expect(AssetDetailBuyCTA.title(tradable: false) == "Unavailable")
    }

    @Test func tradableAssetsOpenTheBuyProposalInTheHostTab() {
        var opened: (route: ProposeFromAssetRoute, tab: MainTab)?

        AssetDetailBuyCTA.open(symbol: "AAPLx", kind: .buy, tab: .cabals) { route, tab in
            opened = (route, tab)
        }

        #expect(opened?.route == ProposeFromAssetRoute(symbol: "AAPLx", kind: .buy))
        #expect(opened?.tab == .cabals)
    }

    @Test func proposeSellOpensTheSellRouteInTheHostTab() {
        var opened: (route: ProposeFromAssetRoute, tab: MainTab)?

        AssetDetailBuyCTA.open(symbol: "GOOGLx", kind: .sell, tab: .home) { route, tab in
            opened = (route, tab)
        }

        #expect(opened?.route == ProposeFromAssetRoute(symbol: "GOOGLx", kind: .sell))
        #expect(opened?.tab == .home)
    }

    @Test func proposeFromTheStocksTabStaysInStocks() {
        var opened: (route: ProposeFromAssetRoute, tab: MainTab)?

        AssetDetailBuyCTA.open(symbol: "AAPLx", kind: .buy, tab: .stocks) { route, tab in
            opened = (route, tab)
        }

        #expect(opened?.route == ProposeFromAssetRoute(symbol: "AAPLx", kind: .buy))
        #expect(opened?.tab == .stocks)
    }

    @Test func proposeSellShowsWhenAVotingCabalHoldsTheAsset() async {
        let model = Self.model(
            cabals: .json(.ok, Self.cabals([("c-1", false), ("c-2", true)])),
            held: [.json(.ok, Self.held(["c-2"]))]
        )

        await model.loadInYourCabals()

        #expect(model.heldByVotingCabal)
        #expect(model.positions.map(\.cabalID) == ["c-2"])
    }

    @Test func proposeSellHidesWhenNoVotingCabalHoldsTheAsset() async {
        let model = Self.model(
            cabals: .json(.ok, Self.cabals([("c-1", true)])), held: [.json(.ok, Self.held([]))])

        await model.loadInYourCabals()

        #expect(!model.heldByVotingCabal)
    }

    @Test func proposeSellHidesWhenOnlyANonVotingCabalHoldsTheAsset() async {
        let model = Self.model(
            cabals: .json(.ok, Self.cabals([("c-1", false)])), held: [.json(.ok, Self.held(["c-1"]))])

        await model.loadInYourCabals()

        #expect(!model.heldByVotingCabal)
        #expect(model.positions.map(\.cabalID) == ["c-1"])
        #expect(model.positions.first?.valueMicros == 60_000_000)
    }

    @Test func aHeldReadThatFailsOnARefreshKeepsThePositions() async {
        let cabals = Self.cabals([("c-1", true)])
        let model = Self.model(
            cabals: .json(.ok, cabals), .json(.ok, cabals),
            held: [.json(.ok, Self.held(["c-1"])), .failure(URLError(.notConnectedToInternet))]
        )
        await model.loadInYourCabals()

        await model.loadInYourCabals()

        #expect(model.heldByVotingCabal)
        #expect(model.positions.map(\.cabalID) == ["c-1"])
    }

    @Test func proposeSellHidesOnceTheHeldReadAnswersWithoutTheAsset() async {
        let cabals = Self.cabals([("c-1", true)])
        let model = Self.model(
            cabals: .json(.ok, cabals), .json(.ok, cabals),
            held: [.json(.ok, Self.held(["c-1"])), .json(.ok, Self.held([]))]
        )
        await model.loadInYourCabals()

        await model.loadInYourCabals()

        #expect(!model.heldByVotingCabal)
        #expect(model.positions.isEmpty)
    }

    @Test func proposeSellDoesNotWaitForTheChart() async throws {
        let chart = try Self.encoded(Components.Schemas.AssetChart.oneDay)
        let detail = StubTransport(scripted: [.json(.ok, try Self.encoded(Components.Schemas.AssetDetail.googl))])
        let gate = StubTransport(scripted: [.gate])
        let series = StubTransport(scripted: [.json(.ok, chart), .json(.ok, chart)])
        let cabals = StubTransport(scripted: [.json(.ok, Self.cabals([("c-1", true)]))])
        let held = StubTransport(scripted: [.json(.ok, Self.held(["c-1"]))])
        let model = Self.model(
            transport: RoutedTransport(routes: [
                "/v1/assets": detail, "/v1/assets/GOOGLx/chart": series, "/v1/assets/GOOGLx/chart?range=1D": gate,
                "/v1/me/cabals": cabals, "/v1/me/held": held,
            ]))

        let started = Task { await model.start() }
        await gate.waitForRequests(1)
        await held.waitForRequests(1)
        await Self.until { model.heldByVotingCabal }

        #expect(model.chartPhase == .loading)
        #expect(model.heldByVotingCabal)

        await gate.releaseGate(.json(.ok, chart))
        await started.value

        #expect(await held.sent.count == 1)
    }

    private static func until(_ predicate: () -> Bool) async {
        while !predicate() {
            await withCheckedContinuation { continuation in
                withObservationTracking {
                    _ = predicate()
                } onChange: {
                    continuation.resume()
                }
            }
        }
    }

    private struct RoutedTransport: ClientTransport {
        let routes: [String: StubTransport]

        func send(_ request: HTTPRequest, body: HTTPBody?, baseURL: URL, operationID: String) async throws
            -> (HTTPResponse, HTTPBody?)
        {
            let path = request.path ?? ""
            let route = routes.keys.filter { path.hasPrefix($0) }.max { $0.count < $1.count }
            guard let route, let target = routes[route] else { throw URLError(.unsupportedURL) }
            return try await target.send(request, body: body, baseURL: baseURL, operationID: operationID)
        }
    }

    private static func encoded(_ value: some Encodable) throws -> String {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return String(decoding: try encoder.encode(value), as: UTF8.self)
    }

    private static func model(_ replies: StubTransport.Reply...) -> AssetDetailClientModel {
        model(transport: StubTransport(scripted: replies))
    }

    private static func model(transport: any ClientTransport) -> AssetDetailClientModel {
        AssetDetailClientModel(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            symbol: "GOOGLx", hints: FakeHintSource())
    }

    private static func model(
        cabals: StubTransport.Reply..., held: [StubTransport.Reply]
    ) -> AssetDetailClientModel {
        model(
            transport: RoutedTransport(routes: [
                "/v1/me/cabals": StubTransport(scripted: cabals), "/v1/me/held": StubTransport(scripted: held),
            ]))
    }

    private static func cabals(_ rows: [(id: String, canVote: Bool)]) -> String {
        let body = rows.map { row in
            #"{"id":"\#(row.id)","name":"Cabal","picture_url":null,"role":"member","can_vote":\#(row.canVote),"#
                + #""member_count":2,"joined_at":"2026-10-02T15:00:00Z","pending_request_count":0,"unread_count":0}"#
        }
        return "[\(body.joined(separator: ","))]"
    }

    private static func held(_ ids: [String]) -> String {
        let rows = ids.map { id in
            #"{"cabal":{"id":"\#(id)","name":"Cabal","picture_url":null},"logo_url":null,"holding":{"#
                + #""symbol":"GOOGLx","display_name":"Stock","kind":"equity","units":"0.3000","token_amount":30000000,"#
                + #""price_micros":200000000,"value_micros":60000000,"weight_bps":6000,"cost_basis_micros":60000000,"#
                + #""pnl_micros":0,"avg_entry_micros":"1","first_bought_at":null}}"#
        }
        return #"{"total_value_micros":0,"cabal_count":\#(ids.count),"prices_as_of":"2026-10-04T12:00:00Z","#
            + #""holdings":[\#(rows.joined(separator: ","))]}"#
    }
}
