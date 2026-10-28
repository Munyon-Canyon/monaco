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
    @Test func aPausedStockCannotBeBoughtAndSaysWhy() {
        #expect(!AssetDetailBuyCTA.canBuy(tradable: true, quotable: false))
        #expect(AssetDetailBuyCTA.canBuy(tradable: true, quotable: true))
        #expect(
            AssetDetailBuyCTA.caption(tradable: true, quotable: false, canSell: true) == "Trading paused right now")
    }

    @Test func theBuyButtonSaysProposeABuyAndTurnsToUnavailableWhenTheStockCannotTrade() {
        #expect(AssetDetailBuyCTA.title(tradable: true) == "Propose a buy")
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
            pots: ["c-1": [.json(.ok, Self.pot(holding: "AAPLx"))], "c-2": [.json(.ok, Self.pot(holding: "GOOGLx"))]]
        )

        await model.loadCabalPositions()

        #expect(model.heldByVotingCabal)
        #expect(model.positions.map(\.cabalID) == ["c-2"])
    }

    @Test func proposeSellHidesWhenNoVotingCabalHoldsTheAsset() async {
        let model = Self.model(
            .json(.ok, Self.cabals([("c-1", true)])),
            .json(.ok, Self.pot(holding: "AAPLx"))
        )

        await model.loadCabalPositions()

        #expect(!model.heldByVotingCabal)
    }

    @Test func proposeSellHidesWhenOnlyANonVotingCabalHoldsTheAsset() async {
        let transport = StubTransport(scripted: [
            .json(.ok, Self.cabals([("c-1", false)])), .json(.ok, Self.pot(holding: "GOOGLx")),
        ])
        let model = Self.model(transport: transport)

        await model.loadCabalPositions()

        #expect(!model.heldByVotingCabal)
        #expect(model.positions.map(\.cabalID) == ["c-1"])
        #expect(model.positions.first?.valueMicros == 60_000_000)
        #expect(await transport.sent.count == 2)
    }

    @Test func aPotReadThatFailsOnARefreshKeepsThatCabalsPosition() async {
        let cabals = Self.cabals([("c-1", true), ("c-2", true)])
        let model = Self.model(
            cabals: .json(.ok, cabals), .json(.ok, cabals),
            pots: [
                "c-1": [.json(.ok, Self.pot(holding: "GOOGLx")), .failure(URLError(.notConnectedToInternet))],
                "c-2": [.json(.ok, Self.pot(holding: "AAPLx")), .json(.ok, Self.pot(holding: "AAPLx"))],
            ]
        )
        await model.loadCabalPositions()

        await model.loadCabalPositions()

        #expect(model.heldByVotingCabal)
        #expect(model.positions.map(\.cabalID) == ["c-1"])
    }

    @Test func aPotReadThatFailsLeavesTheOtherCabalsPositionsOnScreen() async {
        let model = Self.model(
            cabals: .json(.ok, Self.cabals([("c-1", true), ("c-2", true)])),
            pots: [
                "c-1": [.failure(URLError(.notConnectedToInternet))], "c-2": [.json(.ok, Self.pot(holding: "GOOGLx"))],
            ]
        )

        await model.loadCabalPositions()

        #expect(model.positions.map(\.cabalID) == ["c-2"])
    }

    @Test func proposeSellHidesOnceEveryPotAnswersWithoutTheAsset() async {
        let model = Self.model(
            .json(.ok, Self.cabals([("c-1", true)])),
            .json(.ok, Self.pot(holding: "GOOGLx")),
            .json(.ok, Self.cabals([("c-1", true)])),
            .json(.ok, Self.pot(holding: "AAPLx"))
        )
        await model.loadCabalPositions()

        await model.loadCabalPositions()

        #expect(!model.heldByVotingCabal)
        #expect(model.positions.isEmpty)
    }

    @Test func thePotsAreReadTogether() async {
        let first = StubTransport(scripted: [.gate])
        let second = StubTransport(scripted: [.json(.ok, Self.pot(holding: "GOOGLx"))])
        let list = StubTransport(scripted: [.json(.ok, Self.cabals([("c-1", true), ("c-2", true)]))])
        let model = Self.model(
            transport: RoutedTransport(routes: [
                "/v1/me/cabals": list, "/v1/cabals/c-1/pot": first, "/v1/cabals/c-2/pot": second,
            ]))

        let loading = Task { await model.loadCabalPositions() }
        await first.waitForRequests(1)
        await second.waitForRequests(1)

        await first.releaseGate(.json(.ok, Self.pot(holding: "GOOGLx")))
        await loading.value
        #expect(model.positions.map(\.cabalID) == ["c-1", "c-2"])
    }

    @Test func proposeSellDoesNotWaitForTheChart() async throws {
        let chart = try Self.encoded(Components.Schemas.AssetChart.oneDay)
        let detail = StubTransport(scripted: [.json(.ok, try Self.encoded(Components.Schemas.AssetDetail.googl))])
        let held = StubTransport(scripted: [.gate])
        let series = StubTransport(scripted: [.json(.ok, chart), .json(.ok, chart)])
        let cabals = StubTransport(scripted: [
            .json(.ok, Self.cabals([("c-1", true)])), .json(.ok, Self.pot(holding: "GOOGLx")),
        ])
        let model = Self.model(
            transport: RoutedTransport(routes: [
                "/v1/assets": detail, "/v1/assets/GOOGLx/chart": series, "/v1/assets/GOOGLx/chart?range=1D": held,
                "/v1/me/cabals": cabals, "/v1/cabals": cabals,
            ]))

        let started = Task { await model.start() }
        await held.waitForRequests(1)
        await cabals.waitForRequests(2)
        await Self.until { model.heldByVotingCabal }

        #expect(model.chartPhase == .loading)
        #expect(model.heldByVotingCabal)

        await held.releaseGate(.json(.ok, chart))
        await started.value

        #expect(await cabals.sent.count == 2)
    }

    @Test func theCaptionNamesWhatTheCabalVotesOn() {
        #expect(
            AssetDetailBuyCTA.caption(tradable: true, canSell: false) == "Your cabal votes before anything is bought")
        #expect(
            AssetDetailBuyCTA.caption(tradable: true, canSell: true)
                == "Your cabal votes before anything is bought or sold")
    }

    @Test func anUntradableAssetSaysWhyInsteadOfPromisingAVote() {
        #expect(AssetDetailBuyCTA.caption(tradable: false, canSell: false) == "Not available to buy yet.")
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
        cabals: StubTransport.Reply..., pots: [String: [StubTransport.Reply]]
    ) -> AssetDetailClientModel {
        var routes = ["/v1/me/cabals": StubTransport(scripted: cabals)]
        for (id, replies) in pots { routes["/v1/cabals/\(id)/pot"] = StubTransport(scripted: replies) }
        return model(transport: RoutedTransport(routes: routes))
    }

    private static func cabals(_ rows: [(id: String, canVote: Bool)]) -> String {
        let body = rows.map { row in
            #"{"id":"\#(row.id)","name":"Cabal","picture_url":null,"role":"member","can_vote":\#(row.canVote),"#
                + #""member_count":2,"joined_at":"2026-10-02T15:00:00Z","pending_request_count":0,"unread_count":0}"#
        }
        return "[\(body.joined(separator: ","))]"
    }

    private static func pot(holding symbol: String) -> String {
        #"{"cabal_id":"c-2","pot_value_micros":100000000,"cash_micros":40000000,"cash_weight_bps":4000,"#
            + #""pnl_micros":0,"return_bps":null,"prices_as_of":"2026-10-04T12:00:00Z","holdings":[{"#
            + #""symbol":"\#(symbol)","display_name":"Stock","kind":"equity","units":"0.3000","token_amount":30000000,"#
            + #""price_micros":200000000,"value_micros":60000000,"weight_bps":6000,"cost_basis_micros":60000000,"#
            + #""pnl_micros":0}],"me":{"share_units":1,"value_micros":1,"slice_bps":10000,"#
            + #""net_contributed_micros":1,"pnl_micros":0}}"#
    }
}
