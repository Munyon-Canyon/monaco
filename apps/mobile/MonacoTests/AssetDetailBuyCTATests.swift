import MonacoAPI
import MonacoCore
import Testing

@testable import Monaco

@MainActor
struct AssetDetailBuyCTATests {
    @Test func untradableAssetsShowTheDisabledCopy() {
        #expect(AssetDetailBuyCTA.title(tradable: false) == "Can't buy right now")
    }

    @Test func tradableAssetsOpenTheBuyProposalInStocks() {
        var opened: (route: ProposeFromAssetRoute, tab: MainTab)?

        AssetDetailBuyCTA.open(symbol: "AAPLx", kind: .buy) { route, tab in
            opened = (route, tab)
        }

        #expect(opened?.route == ProposeFromAssetRoute(symbol: "AAPLx", kind: .buy))
        #expect(opened?.tab == .stocks)
    }

    @Test func proposeSellOpensTheSellRouteInStocks() {
        var opened: (route: ProposeFromAssetRoute, tab: MainTab)?

        AssetDetailBuyCTA.open(symbol: "GOOGLx", kind: .sell) { route, tab in
            opened = (route, tab)
        }

        #expect(opened?.route == ProposeFromAssetRoute(symbol: "GOOGLx", kind: .sell))
        #expect(opened?.tab == .stocks)
    }

    @Test func proposeSellShowsWhenAVotingCabalHoldsTheAsset() async {
        let model = Self.model(
            .json(.ok, Self.cabals([("c-1", false), ("c-2", true)])),
            .json(.ok, Self.pot(holding: "GOOGLx"))
        )

        await model.loadHeldByVotingCabal()

        #expect(model.heldByVotingCabal)
    }

    @Test func proposeSellHidesWhenNoVotingCabalHoldsTheAsset() async {
        let model = Self.model(
            .json(.ok, Self.cabals([("c-1", true)])),
            .json(.ok, Self.pot(holding: "AAPLx"))
        )

        await model.loadHeldByVotingCabal()

        #expect(!model.heldByVotingCabal)
    }

    @Test func proposeSellHidesWhenOnlyANonVotingCabalHoldsTheAsset() async {
        let transport = StubTransport(scripted: [.json(.ok, Self.cabals([("c-1", false)]))])
        let model = Self.model(transport: transport)

        await model.loadHeldByVotingCabal()

        #expect(!model.heldByVotingCabal)
        #expect(await transport.sent.count == 1)
    }

    private static func model(_ replies: StubTransport.Reply...) -> AssetDetailClientModel {
        model(transport: StubTransport(scripted: replies))
    }

    private static func model(transport: StubTransport) -> AssetDetailClientModel {
        AssetDetailClientModel(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            symbol: "GOOGLx", hints: FakeHintSource())
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
