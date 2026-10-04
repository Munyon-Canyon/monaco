import Foundation
import HTTPTypes
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class CabalEditModelTests: XCTestCase {
    static let cabalID = "01890a5d-ac96-774b-bcce-b302099a8058"
    static let creatorID = "01890a5d-ac96-774b-bcce-b302099a8059"

    func testLoadReadsTheCabalAndSeesTheCreator() async {
        let (model, transport, _) = make([.json(.ok, Self.cabal(name: "QA pot", me: Self.creator))])

        await model.load()

        XCTAssertEqual(model.cabal?.name, "QA pot")
        XCTAssertTrue(model.isCreator)
        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.method), [.get])
        XCTAssertEqual(sent.first?.path, "/v1/cabals/\(Self.cabalID)")
    }

    func testAMemberWhoDidNotCreateTheCabalIsNotTheCreator() async {
        let (model, _, _) = make([.json(.ok, Self.cabal(name: "QA pot", me: #"{"role":"member","can_vote":true}"#))])

        await model.load()

        XCTAssertFalse(model.isCreator)
    }

    func testANonMemberIsNotTheCreator() async {
        let (model, _, _) = make([.json(.ok, Self.cabal(name: "QA pot", me: "null"))])

        await model.load()

        XCTAssertNotNil(model.cabal)
        XCTAssertFalse(model.isCreator)
    }

    func testSaveSendsOnlyTheChangedFieldsAndShowsTheSavedCabal() async throws {
        let (model, transport, _) = make([
            .json(.ok, Self.cabal(name: "QA pot", me: Self.creator)),
            .json(.ok, Self.cabal(name: "QA pot 2", me: Self.creator)),
        ])
        await model.load()
        var edited = try XCTUnwrap(model.settings)
        edited.name = "QA pot 2"
        edited.threshold = "majority"

        let outcome = await model.save(edited)

        XCTAssertEqual(outcome, .saved)
        XCTAssertEqual(model.cabal?.name, "QA pot 2")
        let sent = await transport.sent
        let keyHeader = try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
        XCTAssertEqual(sent.last?.method, .patch)
        XCTAssertNotNil(sent.last?.headerFields[keyHeader])
        let bodies = await transport.sentBodies
        let body = try XCTUnwrap(bodies.last ?? nil)
        let json = try XCTUnwrap(JSONSerialization.jsonObject(with: body) as? [String: String])
        XCTAssertEqual(json, ["name": "QA pot 2", "threshold": "majority"])
    }

    func testSavingNothingSendsNoRequest() async throws {
        let (model, transport, _) = make([.json(.ok, Self.cabal(name: "QA pot", me: Self.creator))])
        await model.load()

        let outcome = await model.save(try XCTUnwrap(model.settings))

        XCTAssertEqual(outcome, .unchanged)
        let count = await transport.sent.count
        XCTAssertEqual(count, 1)
    }

    func testARetryAfterADroppedConnectionReusesTheIdempotencyKey() async throws {
        let (model, transport, _) = make([
            .json(.ok, Self.cabal(name: "QA pot", me: Self.creator)),
            .failure(URLError(.networkConnectionLost)),
            .json(.ok, Self.cabal(name: "QA pot 2", me: Self.creator)),
        ])
        await model.load()
        var edited = try XCTUnwrap(model.settings)
        edited.name = "QA pot 2"

        let first = await model.save(edited)
        let second = await model.save(edited)

        XCTAssertEqual(first, .failed("You're offline. Try again."))
        XCTAssertEqual(second, .saved)
        let keyHeader = try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
        let keys = await transport.sent.dropFirst().map { $0.headerFields[keyHeader] }
        XCTAssertEqual(keys.count, 2)
        XCTAssertEqual(keys[0], keys[1])
    }

    func testARefusedSaveShowsTheServerMessage() async throws {
        let problem =
            #"{"type":"about:blank","title":"Forbidden","status":403,"code":"forbidden","#
            + #""message":"Only the cabal's creator can change it.","trace_id":"00000000000000000000000000000000","#
            + #""retryable":false}"#
        let (model, _, _) = make([
            .json(.ok, Self.cabal(name: "QA pot", me: Self.creator)),
            .response(status: .forbidden, contentType: "application/problem+json", body: Data(problem.utf8)),
        ])
        await model.load()
        var edited = try XCTUnwrap(model.settings)
        edited.joinMode = "request"

        let outcome = await model.save(edited)

        XCTAssertEqual(outcome, .failed("Only the cabal's creator can change it."))
        XCTAssertEqual(model.cabal?.rules.joinMode, "open")
    }

    func testAnUpdatedHintForThisCabalReadsItAgain() async {
        let (model, transport, hints) = make([
            .json(.ok, Self.cabal(name: "QA pot", me: Self.creator)),
            .json(.ok, Self.cabal(name: "QA pot 2", me: Self.creator)),
        ])
        await model.load()
        let observer = Task { await model.observe() }
        addTeardownBlock { observer.cancel() }
        let subscribed = await waitUntil { await hints.subscriberCount == 2 }
        XCTAssertTrue(subscribed)

        await hints.send(.changed(.cabal("someone-else"), what: "updated", id: "1"))
        await hints.send(.changed(.cabal(Self.cabalID), what: "updated", id: "2"))

        let refreshed = await waitUntil { model.cabal?.name == "QA pot 2" }
        XCTAssertTrue(refreshed)
        let count = await transport.sent.count
        XCTAssertEqual(count, 2)
    }

    func testAFailedFirstReadShowsTheFailure() async {
        let (model, _, _) = make([.failure(URLError(.notConnectedToInternet))])

        await model.load()

        guard case .failed(.transport) = model.state else {
            return XCTFail("want a transport failure, got \(model.state)")
        }
        XCTAssertNil(model.settings)
    }

    func testAFailedRefreshKeepsTheCabalOnScreen() async {
        let (model, _, _) = make([
            .json(.ok, Self.cabal(name: "QA pot", me: Self.creator)),
            .failure(URLError(.networkConnectionLost)),
        ])
        await model.load()

        await model.load()

        XCTAssertEqual(model.cabal?.name, "QA pot")
        XCTAssertEqual(model.failureTick, 1)
        XCTAssertEqual(model.lastError, .transport(URLError(.networkConnectionLost)))
    }

    func make(
        _ replies: [StubTransport.Reply]
    ) -> (CabalEditModel, StubTransport, FakeHintStream) {
        let transport = StubTransport(scripted: replies)
        let hints = FakeHintStream()
        let model = CabalEditModel(
            cabalID: Self.cabalID,
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            hints: hints
        )
        return (model, transport, hints)
    }

    func waitUntil(_ predicate: @escaping () async -> Bool) async -> Bool {
        for _ in 0..<500 {
            if await predicate() { return true }
            await Task.yield()
        }
        return await predicate()
    }

    static let creator = #"{"role":"creator","can_vote":true}"#
    static let jordanID = "01890a5d-ac96-774b-bcce-b302099a8061"
    static let priyaID = "01890a5d-ac96-774b-bcce-b302099a8062"

    static func cabal(name: String, me: String, voters: [String]?, members: [String]? = nil) -> String {
        let ids = members ?? [creatorID, jordanID, priyaID]
        let names = [creatorID: "kai", jordanID: "jordan", priyaID: "priya"]
        let rows = ids.map { id in
            let handle = names[id] ?? "someone"
            let canVote = voters.map { $0.contains(id) } ?? true
            return ##"{"user_id":"\##(id)","handle":"\##(handle)","display_name":"\##(handle.capitalized)","##
                + ##""photo_url":null,"role":"\##(id == creatorID ? "creator" : "member")","##
                + ##""can_vote":\##(canVote),"joined_at":"2026-09-30T12:00:00Z"}"##
        }
        let mode = voters == nil ? "all" : "list"
        let memberList = rows.joined(separator: ",")
        return cabal(name: name, me: me)
            .replacingOccurrences(of: #""voter_mode":"all""#, with: #""voter_mode":"\#(mode)""#)
            .replacingOccurrences(
                of: #""member_count":2,"members":[]"#,
                with: #""member_count":\#(ids.count),"members":[\#(memberList)]"#
            )
    }

    static func cabal(name: String, me: String) -> String {
        ##"{"id":"\##(cabalID)","name":"\##(name)","picture_url":null,"status":"active","##
            + ##""rules":{"join_mode":"open","voter_mode":"all","threshold":"unanimous","##
            + ##""proposal_expiry_seconds":86400,"slippage_bps":100},"##
            + ##""creator":{"user_id":"\##(creatorID)","handle":"kai","display_name":"Kai","photo_url":null},"##
            + ##""member_count":2,"members":[],"me":\##(me),"my_access_request":null,"##
            + ##""invite_code":null,"treasury_address":"Dht9c9YfstFWkNYXgqr8HZbhqVn563bCpNU6zL32Ftqf"}"##
    }
}
