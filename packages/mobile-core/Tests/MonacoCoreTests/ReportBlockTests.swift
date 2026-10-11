import HTTPTypes
import MonacoAPI
import MonacoCore
import MonacoFlows
import MonacoTestSupport
import XCTest

@MainActor
final class ReportBlockTests: XCTestCase {
    private func problem(_ status: Int, _ code: String, _ message: String) -> StubTransport.Reply {
        .response(
            status: .init(code: status), contentType: "application/problem+json",
            body: Data(
                (#"{"type":"about:blank","title":"Error","status":\#(status),"code":"\#(code)","#
                    + #""message":"\#(message)","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","retryable":false}"#)
                    .utf8))
    }

    private func model(_ replies: StubTransport.Reply...) -> (Flow20aReportBlockModel, StubTransport) {
        let transport = StubTransport(scripted: replies)
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        return (Flow20aReportBlockModel(api: api, hints: FakeHintStream()), transport)
    }

    private func key(_ request: HTTPRequest) throws -> String? {
        request.headerFields[try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))]
    }

    func testEachReasonSendsItsWireValueAndKind() async throws {
        XCTAssertEqual(ReportReason.allCases.map(\.rawValue), ["spam", "abuse", "other"])
        XCTAssertEqual(ReportReason.allCases.map(\.label), ["Spam", "Abusive", "Something else"])
        for (reason, wire) in zip(ReportReason.allCases, ["spam", "abuse", "other"]) {
            let (model, transport) = model(.json(.created, #"{"id":"01920000-0000-7000-8000-000000000009"}"#))

            let outcome = await model.report(kind: .message, targetID: "m-1", reason: reason)

            XCTAssertEqual(outcome, .ok)
            let requests = await transport.sent
            let request = try XCTUnwrap(requests.first)
            XCTAssertEqual(request.method, .post)
            XCTAssertEqual(request.path, "/v1/reports")
            let bodies = await transport.sentBodies
            let sentBody = try XCTUnwrap(bodies.first ?? nil)
            let json = try XCTUnwrap(JSONSerialization.jsonObject(with: sentBody) as? [String: String])
            XCTAssertEqual(json, ["kind": "message", "target_id": "m-1", "reason": wire])
        }
    }

    func testEveryKindReportsToItsWireValue() async throws {
        for kind in ReportKind.allCases {
            let (model, transport) = model(.json(.created, #"{"id":"01920000-0000-7000-8000-000000000009"}"#))

            _ = await model.report(kind: kind, targetID: "t-1", reason: .spam)

            let bodies = await transport.sentBodies
            let sentBody = try XCTUnwrap(bodies.first ?? nil)
            let json = try XCTUnwrap(JSONSerialization.jsonObject(with: sentBody) as? [String: String])
            XCTAssertEqual(json["kind"], kind.rawValue)
        }
    }

    func testBlockAndUnblockHitTheirRoutes() async throws {
        let (model, transport) = model(
            .response(status: .noContent, contentType: "text/plain", body: Data()),
            .response(status: .noContent, contentType: "text/plain", body: Data()))

        let blocked = await model.block(userID: "u-1")
        let unblocked = await model.unblock(userID: "u-1")

        XCTAssertEqual(blocked, .ok)
        XCTAssertEqual(unblocked, .ok)
        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.method), [.post, .delete])
        XCTAssertEqual(sent.map(\.path), ["/v1/users/u-1/block", "/v1/users/u-1/block"])
    }

    func testEachOutcomeMapsToItsToast() async {
        let cases: [(Int, String, Flow20aOutcome)] = [
            (400, "cannot_block_self", .cannotBlockSelf),
            (404, "user_not_found", .userNotFound),
            (404, "report_target_not_found", .reportTargetNotFound),
            (429, "rate_limited", .rateLimited),
            (401, "unauthorized", .unauthorized),
        ]
        for (status, code, expected) in cases {
            let (model, _) = model(problem(status, code, "Server says \(code)."))

            let outcome = await model.block(userID: "u-1")

            XCTAssertEqual(outcome, expected)
            let expectedToast = expected == .unauthorized ? "Please sign in again." : "Server says \(code)."
            XCTAssertEqual(model.toast(for: outcome, after: .block(handle: "qa_b")), expectedToast)
        }
    }

    func testAnInterruptedSendIsTheOfflineToast() async {
        let (model, _) = model(.failure(URLError(.notConnectedToInternet)))

        let outcome = await model.report(kind: .user, targetID: "u-1", reason: .abuse)

        XCTAssertEqual(outcome, .interrupted)
        XCTAssertEqual(model.toast(for: outcome, after: .report), "You're offline. Try again.")
    }

    func testSuccessToastsPerAction() {
        let (model, _) = model()

        XCTAssertEqual(model.toast(for: .ok, after: .report), "Thanks. We'll take a look.")
        XCTAssertEqual(
            model.toast(for: .ok, after: .block(handle: "qa_b")),
            "Blocked @qa_b. You won't see their messages or comments.")
        XCTAssertEqual(model.toast(for: .ok, after: .unblock(handle: "qa_b")), "Unblocked @qa_b.")
    }

    func testARetryAfterATransportErrorReusesTheIdempotencyKey() async throws {
        let (model, transport) = model(
            .failure(URLError(.networkConnectionLost)),
            .json(.created, #"{"id":"01920000-0000-7000-8000-000000000009"}"#))

        let first = await model.report(kind: .comment, targetID: "c-1", reason: .spam)
        let second = await model.report(kind: .comment, targetID: "c-1", reason: .spam)

        XCTAssertEqual(first, .interrupted)
        XCTAssertEqual(second, .ok)
        let sent = await transport.sent
        XCTAssertEqual(sent.count, 2)
        XCTAssertNotNil(try key(sent[0]))
        XCTAssertEqual(try key(sent[0]), try key(sent[1]))
    }

    func testAFinalRefusalMintsANewKeyForTheNextReport() async throws {
        let (model, transport) = model(
            problem(404, "report_target_not_found", "Gone."),
            .json(.created, #"{"id":"01920000-0000-7000-8000-000000000009"}"#))

        _ = await model.report(kind: .comment, targetID: "c-1", reason: .spam)
        _ = await model.report(kind: .comment, targetID: "c-1", reason: .spam)

        let sent = await transport.sent
        XCTAssertNotEqual(try key(sent[0]), try key(sent[1]))
    }

    func testUnblockFailureRestoresTheRow() async {
        let transport = StubTransport(
            scripted: [
                .json(
                    .ok,
                    #"{"users":[{"user_id":"u-1","handle":"maya","display_name":"Maya","photo_url":null},"#
                        + #"{"user_id":"u-2","handle":"kai","display_name":"Kai","photo_url":null}]}"#),
                .failure(URLError(.notConnectedToInternet)),
            ])
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        let people = BlockedPeopleModel(api: api, hints: FakeHintStream())
        await people.load()

        let outcome = await people.unblock(userID: "u-1")

        XCTAssertEqual(outcome, .interrupted)
        guard case .loaded(let rows) = people.state else { return XCTFail("want rows, got \(people.state)") }
        XCTAssertEqual(rows.map(\.id), ["u-1", "u-2"])
    }

    func testUnblockSuccessRemovesTheRow() async {
        let transport = StubTransport(
            scripted: [
                .json(
                    .ok,
                    #"{"users":[{"user_id":"u-1","handle":"maya","display_name":"Maya","photo_url":null}]}"#),
                .response(status: .noContent, contentType: "text/plain", body: Data()),
            ])
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        let people = BlockedPeopleModel(api: api, hints: FakeHintStream())
        await people.load()

        let outcome = await people.unblock(userID: "u-1")

        XCTAssertEqual(outcome, .ok)
        guard case .loaded(let rows) = people.state else { return XCTFail("want rows, got \(people.state)") }
        XCTAssertTrue(rows.isEmpty)
    }

    #if DEBUG
    func testTheFixtureListsTwoBlockedUsers() {
        XCTAssertEqual(Components.Schemas.BlockedUsers.sample.users.map(\.handle), ["qa_b", "kai"])
        XCTAssertTrue(Components.Schemas.BlockedUsers.sampleEmpty.users.isEmpty)
    }
    #endif
}
