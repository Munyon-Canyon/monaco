import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class ProposeFromAssetModelTests: XCTestCase {
    func testNoCabalWhereTheMemberCanVoteSendsThemToJoin() async {
        let model = makeModel([cabal("a", canVote: false)])

        await model.load()

        XCTAssertEqual(model.destination, .join)
    }

    func testOneVotingCabalGoesStraightToAmount() async {
        let model = makeModel([cabal("a", canVote: true), cabal("b", canVote: false)])

        await model.load()

        XCTAssertEqual(model.destination, .amount("a"))
    }

    func testTwoVotingCabalsAskWhichOne() async {
        let model = makeModel([cabal("a", canVote: true), cabal("b", canVote: true)])

        await model.load()

        XCTAssertEqual(model.destination, .pick)
    }

    func testAFailedReadHasNoDestination() async {
        let model = ProposeFromAssetModel(api: api(StubTransport(.json(.internalServerError, "{}"))))

        await model.load()

        XCTAssertNil(model.destination)
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
