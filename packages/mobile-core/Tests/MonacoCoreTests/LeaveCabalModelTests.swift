import Foundation
import HTTPTypes
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class LeaveCabalModelTests: XCTestCase {
    private let cabalID = "01890a5d-ac96-774b-bcce-b302099a8058"
    private let otherID = "01890a5d-ac96-774b-bcce-b302099a8059"

    func testAMemberCanLeave() async {
        let model = makeModel(StubTransport(myCabals(role: "member", members: 3)))

        await model.load()

        XCTAssertEqual(model.standing, .loaded(LeaveStanding(cabalName: "QA pot", canLeave: true)))
    }

    func testTheCreatorCannotLeaveWhileOthersRemain() async {
        let model = makeModel(StubTransport(myCabals(role: "creator", members: 2)))

        await model.load()

        XCTAssertEqual(model.standing, .loaded(LeaveStanding(cabalName: "QA pot", canLeave: false)))
    }

    func testTheCreatorCanLeaveOnceAlone() async {
        let model = makeModel(StubTransport(myCabals(role: "creator", members: 1)))

        await model.load()

        XCTAssertEqual(model.standing, .loaded(LeaveStanding(cabalName: "QA pot", canLeave: true)))
    }

    func testANonMemberHasNoStanding() async {
        let model = makeModel(StubTransport(.json(.ok, "[\(row(id: otherID, role: "member", members: 2))]")))

        await model.load()

        XCTAssertEqual(model.standing, .loaded(nil))
    }

    func testAFailedReadFails() async throws {
        let model = makeModel(try StubTransport(problem(.upstreamUnavailable, status: 503, "Something broke.")))

        await model.load()

        guard case .failed(.problem(let problem)) = model.standing else {
            return XCTFail("expected a problem, got \(model.standing)")
        }
        XCTAssertEqual(problem.message, "Something broke.")
    }

    func testAStaleReadDoesNotOverwriteANewerOne() async {
        let transport = StubTransport(scripted: [.gate, myCabals(role: "creator", members: 1)])
        let model = makeModel(transport)
        let first = Task { await model.load() }
        let parked = await waitUntil { await transport.sent.count == 1 }

        await model.load()
        await transport.releaseGate(myCabals(role: "creator", members: 3))
        await first.value

        XCTAssertTrue(parked)
        XCTAssertEqual(model.standing, .loaded(LeaveStanding(cabalName: "QA pot", canLeave: true)))
    }

    func testAStaleFailureDoesNotOverwriteANewerRead() async {
        let transport = StubTransport(scripted: [.gate, myCabals(role: "member", members: 2)])
        let model = makeModel(transport)
        let first = Task { await model.load() }
        let parked = await waitUntil { await transport.sent.count == 1 }

        await model.load()
        await transport.releaseGate(.failure(URLError(.networkConnectionLost)))
        await first.value

        XCTAssertTrue(parked)
        XCTAssertEqual(model.standing, .loaded(LeaveStanding(cabalName: "QA pot", canLeave: true)))
    }

    func testLeaveDeletesTheMembershipOnceAndNamesTheCabal() async throws {
        let transport = StubTransport(scripted: [myCabals(role: "member", members: 2), deleted()])
        let model = makeModel(transport)
        await model.load()

        let outcome = await model.leave()

        XCTAssertEqual(outcome, .left(cabalName: "QA pot"))
        let deletes = await transport.sent.filter { $0.method == .delete }
        XCTAssertEqual(deletes.map(\.path), ["/v1/cabals/\(cabalID)/members/me"])
        XCTAssertNotNil(deletes.first?.headerFields[try idempotencyKey()])
        XCTAssertFalse(model.isLeaving)
    }

    func testARetryAfterATransportErrorReusesTheIdempotencyKey() async throws {
        let transport = StubTransport(scripted: [
            myCabals(role: "member", members: 2),
            .failure(URLError(.networkConnectionLost)),
            deleted(),
        ])
        let model = makeModel(transport)
        await model.load()

        let first = await model.leave()
        let second = await model.leave()

        XCTAssertEqual(first, .refused(message: "You're offline. Try again."))
        XCTAssertEqual(second, .left(cabalName: "QA pot"))
        let name = try idempotencyKey()
        let keys = await transport.sent.filter { $0.method == .delete }.map { $0.headerFields[name] }
        XCTAssertEqual(keys.count, 2)
        XCTAssertNotNil(keys[0])
        XCTAssertEqual(keys[0], keys[1])
    }

    func testHoldingSharesRoutesToCashOut() async throws {
        let transport = StubTransport(scripted: [
            myCabals(role: "member", members: 2),
            try problem(.leaveHoldsShares, status: 409, "Cash out your stake before you leave."),
        ])
        let model = makeModel(transport)
        await model.load()

        let outcome = await model.leave()

        XCTAssertEqual(outcome, .cashOutFirst(message: "Cash out your stake before you leave."))
    }

    func testTheCreatorWithMembersSendsNothing() async {
        let transport = StubTransport(scripted: [myCabals(role: "creator", members: 3)])
        let model = makeModel(transport)
        await model.load()

        let outcome = await model.leave()

        XCTAssertNil(outcome)
        let sent = await transport.sent
        XCTAssertEqual(sent.count, 1)
    }

    func testLeaveBeforeLoadSendsNothing() async {
        let transport = StubTransport(scripted: [])
        let model = makeModel(transport)

        let outcome = await model.leave()

        XCTAssertNil(outcome)
        let sent = await transport.sent
        XCTAssertTrue(sent.isEmpty)
    }

    func testConcurrentLeavesDeleteOnce() async {
        let transport = StubTransport(scripted: [myCabals(role: "member", members: 2), deleted()])
        let model = makeModel(transport)
        await model.load()

        async let first = model.leave()
        async let second = model.leave()
        let outcomes = await [first, second]

        XCTAssertEqual(outcomes.compactMap { $0 }, [.left(cabalName: "QA pot")])
        let deletes = await transport.sent.filter { $0.method == .delete }
        XCTAssertEqual(deletes.count, 1)
    }

    func testAMembersHintReloadsTheStanding() async {
        let transport = StubTransport(scripted: [
            myCabals(role: "creator", members: 2), myCabals(role: "creator", members: 1),
        ])
        let hints = FakeHintStream()
        let model = makeModel(transport, hints: hints)
        await model.load()
        let task = Task { await model.observe() }
        _ = await waitUntil { await hints.subscriberCount > 0 }

        await hints.send(.changed(.cabal(otherID), what: "members", id: "1"))
        await hints.send(.changed(.cabal(cabalID), what: "members", id: "2"))
        let reloaded = await waitUntil { model.standing == .loaded(LeaveStanding(cabalName: "QA pot", canLeave: true)) }
        task.cancel()

        XCTAssertTrue(reloaded)
        let sent = await transport.sent
        XCTAssertEqual(sent.count, 2)
    }

    private func deleted() -> StubTransport.Reply {
        .response(status: .noContent, contentType: "application/json", body: Data())
    }

    private func myCabals(role: String, members: Int) -> StubTransport.Reply {
        .json(
            .ok, "[\(row(id: otherID, role: "member", members: 4)),\(row(id: cabalID, role: role, members: members))]")
    }

    private func row(id: String, role: String, members: Int) -> String {
        #"{"id":"\#(id)","name":"QA pot","picture_url":null,"role":"\#(role)","can_vote":true,"#
            + #""member_count":\#(members),"joined_at":"2026-10-02T15:00:00Z","pending_request_count":0,"unread_count":0}"#
    }

    private func problem(_ code: Components.Schemas.ErrorCode, status: Int, _ message: String) throws
        -> StubTransport.Reply
    {
        try .problem(
            Components.Schemas.Problem(
                _type: .about_colon_blank, title: "Error", status: status, code: code, message: message,
                traceId: "4bf92f3577b34da6a3ce929d0e0e4736", retryable: false
            ))
    }

    private func idempotencyKey() throws -> HTTPField.Name {
        try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
    }

    private func makeModel(_ transport: StubTransport, hints: FakeHintStream = FakeHintStream()) -> LeaveCabalModel {
        LeaveCabalModel(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            hints: hints,
            cabalID: cabalID
        )
    }

    private func waitUntil(_ predicate: @escaping () async -> Bool) async -> Bool {
        for _ in 0..<500 {
            if await predicate() { return true }
            await Task.yield()
        }
        return await predicate()
    }
}
