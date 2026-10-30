import MonacoAPI
import MonacoTestSupport
import XCTest

@testable import MonacoCore

@MainActor
final class HomePendingVotesModelTests: XCTestCase {
    private let pendingPath = "/v1/me/pending-votes"

    private func pending(_ id: String, cabal: String = "cabal-1", closes: String) -> String {
        #"{"proposal_id":"\#(id)","cabal_id":"\#(cabal)","cabal_name":"AC Capital","kind":"buy","rule_change":null,"#
            + #""symbol":"NVDAx","usdc_micros":400000000,"token_amount":null,"asset_name":"NVIDIA","asset_logo_url":null,"#
            + #""proposer":{"user_id":"user-mia","handle":"mia","display_name":"Mia","photo_url":null},"#
            + #""tally":{"yes":4,"no":1,"voters":8,"needed":5},"voted_count":5,"reply_count":6,"#
            + #""created_at":"2033-05-17T09:00:00Z","expires_at":"\#(closes)"}"#
    }

    private func list(_ rows: String...) -> StubTransport.Reply {
        .json(.ok, "[" + rows.joined(separator: ",") + "]")
    }

    private func model(
        _ transport: StubTransport, hints: FakeHintStream = FakeHintStream(), releaseAfter: TimeInterval = 10
    ) -> HomePendingVotesModel {
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token"), transport: transport)
        return HomePendingVotesModel(api: api, hints: hints, releaseAfter: releaseAfter)
    }

    private let cast =
        #"{"proposal_id":"p-1","status":"open","tally":{"yes":5,"no":1,"voters":8,"needed":5},"my_ballot":"yes","undo_until":null,"my_vote":{"order":5,"of":8,"yes_order":5,"no_order":0,"decided":false}}"#

    func testALoadOfTwoPendingVotesMakesExactlyOneRequestAndKeepsSoonestClosingFirst() async {
        let transport = StubTransport(
            routes: [
                pendingPath: [
                    list(
                        pending("p-2", closes: "2033-05-18T15:00:00Z"), pending("p-1", closes: "2033-05-17T15:00:00Z"))
                ]
            ])
        let model = model(transport)
        await model.load()
        let paths = await transport.sent.map(\.path)
        XCTAssertEqual(paths, [pendingPath])
        XCTAssertEqual(model.cards.map(\.id), ["p-1", "p-2"])
        XCTAssertEqual(model.waitingCount, 2)
        XCTAssertEqual(model.phase, .loaded)
    }

    func testAHintReloadsTheList() async {
        let transport = StubTransport(
            routes: [
                pendingPath: [list(), list(pending("p-1", closes: "2033-05-17T15:00:00Z"))]
            ])
        let hints = FakeHintStream()
        let model = model(transport, hints: hints)
        await model.load()
        let observing = Task { await model.observe() }
        await hints.waitForSubscribers(3)
        model.setVisible(true)
        await hints.send(.changed(.cabal("cabal-1"), what: "proposal_created", id: "p-1"))
        await transport.waitForRequests(2)
        for _ in 0..<5_000 where model.cards.isEmpty { await Task.yield() }
        observing.cancel()
        XCTAssertEqual(model.cards.map(\.id), ["p-1"])
    }

    func testACommittedVoteFiresOnVoteCommittedWhileTheCardIsStillListed() async {
        let transport = StubTransport(
            routes: [
                pendingPath: [list(pending("p-1", closes: "2033-05-17T15:00:00Z"))],
                "/v1/proposals/p-1/votes": [.json(.ok, cast)],
            ])
        let model = model(transport)
        var fired: [(String, [String])] = []
        model.onVoteCommitted = { card, outcome in
            fired.append((card.id, model.cards.map(\.id)))
            XCTAssertEqual(outcome.choice, .yes)
        }
        await model.load()
        _ = await model.shown?.vote(.yes)
        XCTAssertEqual(fired.count, 1)
        XCTAssertEqual(fired.first?.0, "p-1")
        XCTAssertEqual(fired.first?.1, ["p-1"])
    }

    func testASettlingVoteCollapsesItsCardToTheSettledLineAndLeavesYourVoteCount() async {
        let settling =
            #"{"proposal_id":"p-1","status":"passed","tally":{"yes":5,"no":1,"voters":8,"needed":5},"my_ballot":"yes","undo_until":null,"my_vote":{"order":5,"of":8,"yes_order":5,"no_order":0,"decided":true}}"#
        let transport = StubTransport(
            routes: [
                pendingPath: [list(pending("p-1", closes: "2033-05-17T15:00:00Z"))],
                "/v1/proposals/p-1/votes": [.json(.ok, settling)],
            ])
        let model = model(transport)
        await model.load()
        XCTAssertEqual(model.waitingCount, 1)
        _ = await model.shown?.vote(.yes)
        let card = model.shown
        XCTAssertEqual(card?.isSettled, true)
        XCTAssertEqual(card?.subhead(now: .now).contains("passed 5\u{2013}1"), true)
        XCTAssertEqual(model.waitingCount, 0)
        XCTAssertEqual(model.votedIDs, ["p-1"])
    }

    func testARefusedVoteDoesNotFireOnVoteCommitted() async {
        let transport = StubTransport(
            routes: [
                pendingPath: [list(pending("p-1", closes: "2033-05-17T15:00:00Z"))],
                "/v1/proposals/p-1/votes": [.json(.conflict, "{}")],
            ])
        let model = model(transport)
        var count = 0
        model.onVoteCommitted = { _, _ in count += 1 }
        await model.load()
        _ = await model.shown?.vote(.yes)
        XCTAssertEqual(count, 0)
    }

    func testTheCommittedCardSurvivesAReloadThatDropsIt() async {
        let transport = StubTransport(
            routes: [
                pendingPath: [list(pending("p-1", closes: "2033-05-17T15:00:00Z")), list()],
                "/v1/proposals/p-1/votes": [.json(.ok, cast)],
            ])
        let model = model(transport)
        await model.load()
        _ = await model.shown?.vote(.yes)
        await model.load()
        XCTAssertEqual(model.cards.map(\.id), ["p-1"])
        XCTAssertEqual(model.votedIDs, ["p-1"])
        XCTAssertEqual(model.waitingCount, 0)
        XCTAssertTrue(model.rows.isEmpty)
    }

    func testTheCommittedCardLeavesOnlyOnceTheUserHasLeftHomeAndTheHoldHasPassed() async {
        let transport = StubTransport(
            routes: [
                pendingPath: [list(pending("p-1", closes: "2033-05-17T15:00:00Z")), list(), list()],
                "/v1/proposals/p-1/votes": [.json(.ok, cast)],
            ])
        let model = model(transport, releaseAfter: 0)
        await model.load()
        _ = await model.shown?.vote(.yes)
        await model.load()
        XCTAssertEqual(model.cards.count, 1)
        model.setVisible(false)
        model.setVisible(true)
        XCTAssertTrue(model.cards.isEmpty)
    }

    func testTheCommittedCardStaysWhileTheHoldHasNotPassed() async {
        let transport = StubTransport(
            routes: [
                pendingPath: [list(pending("p-1", closes: "2033-05-17T15:00:00Z")), list()],
                "/v1/proposals/p-1/votes": [.json(.ok, cast)],
            ])
        let model = model(transport, releaseAfter: 3_600)
        await model.load()
        _ = await model.shown?.vote(.yes)
        await model.load()
        model.setVisible(false)
        model.setVisible(true)
        XCTAssertEqual(model.cards.count, 1)
    }
}
