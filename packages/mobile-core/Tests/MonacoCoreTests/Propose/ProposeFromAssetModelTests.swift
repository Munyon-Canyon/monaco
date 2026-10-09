import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class ProposeFromAssetModelTests: XCTestCase {
    func testAMemberInNoCabalIsSentToJoin() async {
        let model = makeModel([])

        await model.load(kind: .buy, symbol: "AAPLx")

        XCTAssertEqual(model.destination, .join)
    }

    func testOneVotingCabalGoesStraightToAmount() async {
        let model = makeModel([cabal("a", canVote: true), cabal("b", canVote: false)])

        await model.load(kind: .buy, symbol: "AAPLx")

        XCTAssertEqual(model.destination, .amount("a"))
    }

    func testTwoVotingCabalsAskWhichOne() async {
        let model = makeModel([cabal("a", canVote: true), cabal("b", canVote: true)])

        await model.load(kind: .buy, symbol: "AAPLx")

        XCTAssertEqual(model.destination, .pick)
    }

    func testAFailedReadHasNoDestination() async {
        let model = ProposeFromAssetModel(api: api(StubTransport(.json(.internalServerError, "{}"))))

        await model.load(kind: .buy, symbol: "AAPLx")

        XCTAssertNil(model.destination)
    }

    func testASellKeepsOnlyTheCabalWhoseHoldingsIncludeTheSymbol() async {
        let model = ProposeFromAssetModel(
            api: api(
                StubTransport(routes: [
                    "/v1/me/cabals": [
                        .json(.ok, "[" + cabal("a", canVote: true) + "," + cabal("b", canVote: true) + "]")
                    ],
                    "/v1/cabals/a/pot": [.json(.ok, pot(holding: "AAPLx"))],
                    "/v1/cabals/b/pot": [.json(.ok, pot(holding: "TSLAx"))],
                ])))

        await model.load(kind: .sell, symbol: "aaplx")

        XCTAssertEqual(model.destination, .amount("a"))
        guard case .loaded(let cabals) = model.state else { return XCTFail("expected loaded") }
        XCTAssertEqual(cabals.map(\.id), ["a"])
    }

    func testAMemberWithCabalsButNoVoteGetsTheVotersOnlyDestination() async {
        let model = makeModel([cabal("a", canVote: false)])

        await model.load(kind: .sell, symbol: "AAPLx")

        XCTAssertEqual(model.destination, .votersOnly)
    }

    func testATryAgainKeepsTheLoadedCabalsWhileItReads() async {
        let transport = StubTransport(scripted: [
            .json(.ok, "[" + cabal("a", canVote: true) + "]"), .gate,
        ])
        let model = ProposeFromAssetModel(api: api(transport))
        await model.load(kind: .buy, symbol: "AAPLx")

        let reload = Task { await model.load(kind: .buy, symbol: "AAPLx") }
        await transport.waitForRequests(2)

        XCTAssertEqual(model.destination, .amount("a"))
        await transport.releaseGate(.json(.ok, "[]"))
        await reload.value
        XCTAssertEqual(model.destination, .join)
    }

    private func pot(holding symbol: String) -> String {
        #"{"cabal_id":"c","pot_value_micros":1000000,"cash_micros":0,"cash_weight_bps":0,"pnl_micros":0,"return_bps":null,"prices_as_of":"2026-10-03T15:00:00Z","holdings":[{"symbol":"\#(symbol)","display_name":"Stock","kind":"equity","units":"1.0000","token_amount":1000000,"price_micros":1000000,"value_micros":1000000,"weight_bps":10000,"cost_basis_micros":1000000,"pnl_micros":0}],"me":null}"#
    }

    private func makeModel(_ cabals: [String]) -> ProposeFromAssetModel {
        ProposeFromAssetModel(api: api(StubTransport(.json(.ok, "[" + cabals.joined(separator: ",") + "]"))))
    }

    private func api(_ transport: StubTransport) -> APIClient {
        APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
    }

    private func cabal(_ id: String, canVote: Bool) -> String {
        #"{"id":"\#(id)","name":"Cabal \#(id)","picture_url":null,"role":"member","can_vote":\#(canVote),"member_count":2,"joined_at":"2026-09-30T12:00:00Z","pending_request_count":0,"unread_count":0}"#
    }
}
