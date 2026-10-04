import Foundation
import HTTPTypes
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class CabalEntryTests: XCTestCase {
    private let cabalID = "01890a5d-ac96-774b-bcce-b302099a8060"
    private let requestID = "01890a5d-ac96-774b-bcce-b302099a8059"

    func testARequestCabalThatTurnedOpenJoinsInstead() async throws {
        let transport = StubTransport(scripted: [
            Self.problem(409, "request_not_needed", "Just join."),
            .json(.ok, try Self.cabal()),
        ])
        let entry = await api(transport).enterCabal(cabalID, mode: .request, submission: IdempotentSubmission())
        XCTAssertEqual(entry, .joined)
        let paths = await transport.sent.map { $0.path ?? "" }
        XCTAssertEqual(paths, ["/v1/cabals/\(cabalID)/access-requests", "/v1/cabals/\(cabalID)/members"])
    }

    func testAModeThatFlipsTwiceStopsAtTheRefusal() async throws {
        let transport = StubTransport(scripted: [
            Self.problem(409, "join_needs_request", "Ask to join."),
            Self.problem(409, "request_not_needed", "Just join."),
        ])
        let entry = await api(transport).enterCabal(cabalID, mode: .open, submission: IdempotentSubmission())
        guard case .refused(let error) = entry else { return XCTFail("expected a refusal, got \(entry)") }
        XCTAssertEqual(ToastCopy.message(for: error), "Just join.")
        XCTAssertFalse(entry.isMember)
    }

    func testRevokeDeletesTheRequestWithAKey() async throws {
        let transport = StubTransport(.json(.ok, #"{"id":"\#(requestID)","direction":"request","status":"revoked"}"#))
        try await api(transport).revokeAccessRequest(
            cabalID: cabalID, requestID: requestID, submission: IdempotentSubmission())
        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.method), [.delete])
        XCTAssertEqual(sent.first?.path, "/v1/cabals/\(cabalID)/access-requests/\(requestID)")
        XCTAssertNotNil(sent.first?.headerFields[try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))])
    }

    func testDecideSendsApproveOrDeny() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, #"{"id":"\#(requestID)","direction":"request","status":"approved"}"#),
            .json(.ok, #"{"id":"\#(requestID)","direction":"request","status":"denied"}"#),
        ])
        let client = api(transport)
        try await client.decideAccessRequest(
            cabalID: cabalID, requestID: requestID, approve: true, submission: IdempotentSubmission())
        try await client.decideAccessRequest(
            cabalID: cabalID, requestID: requestID, approve: false, submission: IdempotentSubmission())
        let bodies = await transport.sentBodies.compactMap { $0 }
        let decisions = try bodies.map { try JSONSerialization.jsonObject(with: $0) as? [String: String] }
        XCTAssertEqual(decisions, [["decision": "approve"], ["decision": "deny"]])
        let paths = await transport.sent.map { $0.path ?? "" }
        XCTAssertEqual(Set(paths), ["/v1/cabals/\(cabalID)/access-requests/\(requestID)/decision"])
    }

    func testJoinedAndAlreadyMemberAreMembers() {
        XCTAssertTrue(CabalEntry.joined.isMember)
        XCTAssertTrue(CabalEntry.alreadyMember.isMember)
        XCTAssertFalse(CabalEntry.requested.isMember)
        XCTAssertFalse(CabalEntry.requestPending.isMember)
    }

    func testTheJoinPreviewJoinsFromTheFixtures() async {
        let model = JoinCabalModel.preview(joinMode: "request")
        model.code = "ABCD2345XY"
        await model.lookUp()
        XCTAssertEqual(model.actionTitle, "Request to join")
        let joined = await model.submit()
        XCTAssertEqual(joined?.toast, CabalEntry.requestedToast)
        let open = JoinCabalModel.preview(joinMode: "open")
        open.code = "ABCD2345XY"
        let entered = await open.submit()
        XCTAssertEqual(entered?.toast, CabalEntry.joinedToast)
    }

    private func api(_ transport: StubTransport) -> APIClient {
        APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
    }

    private static func cabal() throws -> String {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return String(decoding: try encoder.encode(Components.Schemas.Cabal.sample(role: "member")), as: UTF8.self)
    }

    private static func problem(_ status: Int, _ code: String, _ message: String) -> StubTransport.Reply {
        let body =
            #"{"type":"about:blank","title":"Error","status":\#(status),"code":"\#(code)","message":"\#(message)","#
            + #""trace_id":"00000000000000000000000000000000","retryable":false}"#
        return .response(
            status: .init(code: status), contentType: "application/problem+json", body: Data(body.utf8)
        )
    }
}
