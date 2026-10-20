import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class NotificationPreferencesModelTests: XCTestCase {
    private func model(_ replies: StubTransport.Reply...) -> (NotificationPreferencesModel, StubTransport) {
        let transport = StubTransport(scripted: replies)
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        return (NotificationPreferencesModel(api: api), transport)
    }

    private func body(_ decisions: Bool, _ replies: Bool) -> String {
        #"{"decision_requests":\#(decisions),"replies_and_mentions":\#(replies)}"#
    }

    func testLoadsTheSavedChoices() async {
        let (model, transport) = model(.json(.ok, body(false, true)))

        await model.load()

        XCTAssertEqual(model.choices, NotificationChoices(decisionRequests: false, repliesAndMentions: true))
        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.path), ["/v1/me/notification-preferences"])
    }

    func testAFailedLoadLeavesTheChoicesUnknownAndLocked() async {
        let (model, _) = model(.failure(URLError(.notConnectedToInternet)))

        await model.load()

        XCTAssertNil(model.choices)
        XCTAssertFalse(model.canChange)
        let error = await model.set(.decisionRequests, to: false)
        XCTAssertNil(error)
        XCTAssertNil(model.choices)
    }

    func testSavingSendsBothChoicesAndKeepsTheStoredAnswer() async throws {
        let (model, transport) = model(.json(.ok, body(true, true)), .json(.ok, body(false, true)))
        await model.load()

        let error = await model.set(.decisionRequests, to: false)

        XCTAssertNil(error)
        XCTAssertEqual(model.choices, NotificationChoices(decisionRequests: false, repliesAndMentions: true))
        XCTAssertTrue(model.canChange)
        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.method), [.get, .put])
        let keyHeader = try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
        XCTAssertNotNil(sent.last?.headerFields[keyHeader])
        let bodies = await transport.sentBodies
        let json = try XCTUnwrap(bodies.last ?? nil)
        let sentChoices = try JSONDecoder().decode([String: Bool].self, from: json)
        XCTAssertEqual(sentChoices, ["decision_requests": false, "replies_and_mentions": true])
    }

    func testAFailedSaveRollsBackAndReturnsTheError() async throws {
        let (model, _) = model(.json(.ok, body(true, false)), .failure(URLError(.notConnectedToInternet)))
        await model.load()

        let error = await model.set(.repliesAndMentions, to: true)

        XCTAssertNotNil(error)
        XCTAssertEqual(model.choices, NotificationChoices(decisionRequests: true, repliesAndMentions: false))
        XCTAssertTrue(model.canChange)
    }

    func testAnOptimisticChoiceShowsWhileTheSaveIsInFlight() async {
        let (model, transport) = model(.json(.ok, body(true, true)), .gate)
        await model.load()

        let save = Task { await model.set(.decisionRequests, to: false) }
        await transport.waitForRequests(2)

        XCTAssertEqual(model.choices?.decisionRequests, false)
        XCTAssertFalse(model.canChange)
        await transport.releaseGate(.json(.ok, body(false, true)))
        let error = await save.value
        XCTAssertNil(error)
        XCTAssertTrue(model.canChange)
    }

    func testSettingTheValueItAlreadyHasSendsNothing() async {
        let (model, transport) = model(.json(.ok, body(true, true)))
        await model.load()

        let error = await model.set(.decisionRequests, to: true)

        XCTAssertNil(error)
        let sent = await transport.sent
        XCTAssertEqual(sent.count, 1)
    }
}
