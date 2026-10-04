import Foundation
import MonacoAPI
import MonacoCabal
import MonacoTestSupport
import XCTest

@MainActor
final class CabalActionsModelTests: XCTestCase {
    private static let cabalID = "01890a5d-ac96-774b-bcce-b302099a8060"

    func testLoadReadsTheCabalOnce() async {
        let (model, transport, _) = make([.json(.ok, Self.cabal(me: Self.voter))])

        await model.load()

        XCTAssertEqual(model.actions, .member(canPropose: true))
        XCTAssertEqual(model.cabalName, "QA pot")
        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.method), [.get])
        XCTAssertEqual(sent.first?.path, "/v1/cabals/\(Self.cabalID)")
    }

    func testANonMemberSeesNoActions() async {
        let (model, _, _) = make([.json(.ok, Self.cabal(me: "null"))])

        await model.load()

        XCTAssertEqual(model.actions, .hidden)
    }

    func testAMemberWhoCannotVoteCannotPropose() async {
        let (model, _, _) = make([.json(.ok, Self.cabal(me: #"{"role":"member","can_vote":false}"#))])

        await model.load()

        XCTAssertEqual(model.actions, .member(canPropose: false))
    }

    func testAFailedFirstReadShowsTheFailure() async {
        let (model, _, _) = make([.failure(URLError(.notConnectedToInternet))])

        await model.load()

        guard case .failed(.transport) = model.actions else {
            return XCTFail("want a transport failure, got \(model.actions)")
        }
        XCTAssertNil(model.toast)
    }

    func testAFailedReloadKeepsTheRowAndToasts() async {
        let (model, _, _) = make([
            .json(.ok, Self.cabal(me: Self.voter)),
            .failure(URLError(.networkConnectionLost)),
        ])
        await model.load()

        await model.load()

        XCTAssertEqual(model.actions, .member(canPropose: true))
        XCTAssertEqual(model.toast, "You're offline. Try again.")
        model.dismissToast()
        XCTAssertNil(model.toast)
    }

    func testAMembersHintForThisCabalReadsItAgainAndOneForAnotherCabalDoesNot() async {
        let (model, transport, hints) = make([
            .json(.ok, Self.cabal(me: Self.voter)),
            .json(.ok, Self.cabal(me: #"{"role":"member","can_vote":false}"#)),
        ])
        await model.load()
        let observer = await observing(model, hints)
        addTeardownBlock { observer.cancel() }

        await hints.send(.changed(.cabal("someone-else"), what: "members", id: "1"))
        await hints.send(.changed(.cabal(Self.cabalID), what: "members", id: "2"))

        let refreshed = await waitUntil { model.actions == .member(canPropose: false) }
        XCTAssertTrue(refreshed)
        _ = await waitUntil { false }
        let count = await transport.sent.count
        XCTAssertEqual(count, 2)
    }

    func testAnUnrelatedHintForThisCabalDoesNotReadIt() async {
        let (model, transport, hints) = make([.json(.ok, Self.cabal(me: Self.voter))])
        await model.load()
        let observer = await observing(model, hints)
        addTeardownBlock { observer.cancel() }

        await hints.send(.changed(.cabal(Self.cabalID), what: "pot", id: "1"))

        _ = await waitUntil { false }
        let count = await transport.sent.count
        XCTAssertEqual(count, 1)
    }

    func testUpdatedAndCabalAccessHintsReadItAgain() async {
        let (model, transport, hints) = make([
            .json(.ok, Self.cabal(me: Self.voter)),
            .json(.ok, Self.cabal(me: Self.voter)),
            .json(.ok, Self.cabal(me: "null")),
        ])
        await model.load()
        let observer = await observing(model, hints)
        addTeardownBlock { observer.cancel() }

        await hints.send(.changed(.cabal(Self.cabalID), what: "updated", id: "1"))
        _ = await waitUntil { await transport.sent.count == 2 }
        await hints.send(.changed(.user("me"), what: "cabal_access", id: "2"))

        let hidden = await waitUntil { model.actions == .hidden }
        XCTAssertTrue(hidden)
    }

    func testAResyncReadsItAgainOnce() async {
        let (model, transport, hints) = make([
            .json(.ok, Self.cabal(me: Self.voter)),
            .json(.ok, Self.cabal(me: "null")),
        ])
        await model.load()
        let observer = await observing(model, hints)
        addTeardownBlock { observer.cancel() }

        await hints.send(.resync)

        let hidden = await waitUntil { model.actions == .hidden }
        XCTAssertTrue(hidden)
        _ = await waitUntil { false }
        let count = await transport.sent.count
        XCTAssertEqual(count, 2)
    }

    func testThePreviewShowsTheSampleRole() async {
        let voter = CabalActionsModel.preview(.sample(role: "member", canVote: true))
        let nonvoter = CabalActionsModel.preview(.sample(role: "member", canVote: false))
        let outsider = CabalActionsModel.preview(.sample(role: nil))

        await voter.load()
        await nonvoter.load()
        await outsider.load()

        XCTAssertEqual(voter.actions, .member(canPropose: true))
        XCTAssertEqual(nonvoter.actions, .member(canPropose: false))
        XCTAssertEqual(outsider.actions, .hidden)
    }

    private func make(
        _ replies: [StubTransport.Reply]
    ) -> (CabalActionsModel, StubTransport, FakeHintStream) {
        let transport = StubTransport(scripted: replies)
        let hints = FakeHintStream()
        let model = CabalActionsModel(
            cabalID: Self.cabalID,
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            hints: hints
        )
        return (model, transport, hints)
    }

    private func observing(_ model: CabalActionsModel, _ hints: FakeHintStream) async -> Task<Void, Never> {
        let observer = Task { await model.observe() }
        let subscribed = await waitUntil { await hints.subscriberCount == 2 }
        XCTAssertTrue(subscribed)
        return observer
    }

    private func waitUntil(_ predicate: @escaping () async -> Bool) async -> Bool {
        for _ in 0..<500 {
            if await predicate() { return true }
            await Task.yield()
        }
        return await predicate()
    }

    private static let voter = #"{"role":"member","can_vote":true}"#

    private static func cabal(me: String) -> String {
        ##"{"id":"\##(cabalID)","name":"QA pot","picture_url":null,"status":"active","##
            + ##""rules":{"join_mode":"open","voter_mode":"all","threshold":"unanimous","##
            + ##""proposal_expiry_seconds":86400,"slippage_bps":100},"##
            + ##""creator":{"user_id":"01890a5d-ac96-774b-bcce-b302099a8058","handle":"kai","##
            + ##""display_name":"Kai","photo_url":null},"##
            + ##""member_count":2,"members":[],"me":\##(me),"my_access_request":null,"##
            + ##""invite_code":null,"treasury_address":"Dht9c9YfstFWkNYXgqr8HZbhqVn563bCpNU6zL32Ftqf"}"##
    }
}
