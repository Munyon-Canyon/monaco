import HTTPTypes
import MonacoAPI
import MonacoTestSupport
import XCTest

@testable import MonacoCore

@MainActor
final class ProposalRemindModelTests: XCTestCase {
    private func reply(set: Bool, due: String? = "2026-10-04T14:00:00Z") -> StubTransport.Reply {
        .json(
            .ok,
            #"{"proposal_id":"proposal-1","reminder_set":\#(set),"due_at":\#(due.map { "\"\($0)\"" } ?? "null")}"#)
    }

    private func repository(_ transport: StubTransport) -> ProposalsRepository {
        ProposalsRepository(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token"), transport: transport))
    }

    private func model(_ transport: StubTransport, initial: Bool = false) -> ProposalRemindModel {
        ProposalRemindModel(proposalID: "proposal-1", initial: initial, repository: repository(transport))
    }

    private func key(_ request: HTTPRequest) throws -> String? {
        request.headerFields[try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))]
    }

    func testTurningItOnPutsTheReminder() async throws {
        let transport = StubTransport(reply(set: true))
        let model = model(transport)
        let state = await model.toggle()
        XCTAssertTrue(model.isOn)
        XCTAssertFalse(model.isSending)
        XCTAssertEqual(state?.isSet, true)
        XCTAssertEqual(state?.dueAt, Date(timeIntervalSince1970: 1_791_122_400))
        let sent = await transport.sent
        let request = try XCTUnwrap(sent.first)
        XCTAssertEqual(request.path, "/v1/proposals/proposal-1/reminder")
        XCTAssertEqual(request.method, .put)
        XCTAssertNotNil(try key(request))
    }

    func testTurningItOffDeletesTheReminder() async throws {
        let transport = StubTransport(reply(set: false, due: nil))
        let model = model(transport, initial: true)
        let state = await model.toggle()
        XCTAssertFalse(model.isOn)
        XCTAssertNil(state?.dueAt)
        let sent = await transport.sent
        let request = try XCTUnwrap(sent.first)
        XCTAssertEqual(request.method, .delete)
    }

    func testAFailureRevertsWithTheServerMessage() async {
        let model = model(StubTransport(.failure(URLError(.notConnectedToInternet))))
        let state = await model.toggle()
        XCTAssertNil(state)
        XCTAssertFalse(model.isOn)
        XCTAssertEqual(model.errorMessage, "You're offline. Try again.")
    }

    func testAFailedOffRevertsToOn() async {
        let model = model(StubTransport(.failure(URLError(.notConnectedToInternet))), initial: true)
        _ = await model.toggle()
        XCTAssertTrue(model.isOn)
    }

    func testARetryReusesTheKeyAndASuccessTakesANewOne() async throws {
        let transport = StubTransport(scripted: [
            .json(.internalServerError, #"{"message":"Try again"}"#), reply(set: true), reply(set: false, due: nil),
            reply(set: true),
        ])
        let model = model(transport)
        _ = await model.toggle()
        XCTAssertNotNil(model.errorMessage)
        _ = await model.toggle()
        XCTAssertNil(model.errorMessage)
        _ = await model.toggle()
        _ = await model.toggle()
        let sent = await transport.sent
        let keys = try sent.map { try XCTUnwrap(key($0)) }
        XCTAssertEqual(keys[0], keys[1])
        XCTAssertNotEqual(keys[1], keys[3])
    }

    func testApplyFollowsTheServerWhenIdle() {
        let model = model(StubTransport(reply(set: true)), initial: true)
        model.apply(server: false)
        XCTAssertFalse(model.isOn)
        model.apply(server: true)
        XCTAssertTrue(model.isOn)
    }

    func testApplyIsIgnoredWhileASendIsInFlightAndASecondToggleIsDropped() async {
        let transport = StubTransport(scripted: [.gate])
        let model = model(transport)
        let sending = Task { await model.toggle() }
        await transport.waitForRequest()
        XCTAssertTrue(model.isSending)
        model.apply(server: false)
        XCTAssertTrue(model.isOn)
        let second = await model.toggle()
        XCTAssertNil(second)
        await transport.releaseGate(reply(set: true))
        _ = await sending.value
        XCTAssertTrue(model.isOn)
        model.apply(server: false)
        XCTAssertFalse(model.isOn)
    }

    func testToastCopy() {
        let now = Date(timeIntervalSince1970: 1_791_000_000)
        XCTAssertEqual(
            ProposalRemindModel.toastAfterOn(dueAt: now.addingTimeInterval(3_600), now: now),
            "We'll remind you an hour before it closes.")
        XCTAssertEqual(
            ProposalRemindModel.toastAfterOn(dueAt: now.addingTimeInterval(30), now: now),
            "It closes within the hour. Reminder on.")
        XCTAssertEqual(
            ProposalRemindModel.toastAfterOn(dueAt: nil, now: now), "We'll remind you an hour before it closes.")
        XCTAssertEqual(ProposalRemindModel.offToast, "Reminder off.")
    }

    func testALoadedReminderSetTrueRendersOnAndALaterFalseTurnsItOff() async throws {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        var detail = Components.Schemas.ProposalDetail.sample(canVote: true)
        detail.reminderSet = true
        XCTAssertTrue(ProposalDetail(detail).reminderSet)
        let on = String(decoding: try encoder.encode(detail), as: UTF8.self)
        detail.reminderSet = false
        let off = String(decoding: try encoder.encode(detail), as: UTF8.self)
        let transport = StubTransport(routes: ["/v1/proposals/proposal-1": [.json(.ok, on), .json(.ok, off)]])
        let detailModel = ProposalDetailModel(
            id: "proposal-1", repository: repository(transport), hints: FakeHintStream())
        XCTAssertNil(detailModel.remind)
        await detailModel.load()
        XCTAssertEqual(detailModel.remind?.isOn, true)
        await detailModel.load()
        XCTAssertEqual(detailModel.remind?.isOn, false)
    }
}
