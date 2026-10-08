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

    func testEnteringFilesAnAccessRequestAndNeverJoins() async throws {
        let pending = #"{"id":"\#(requestID)","direction":"request","status":"pending"}"#
        let transport = StubTransport(.json(.created, pending))
        let entry = await api(transport).enterCabal(cabalID, submission: IdempotentSubmission())
        XCTAssertEqual(entry, .requested)
        XCTAssertFalse(entry.isMember)
        let paths = await transport.sent.map { $0.path ?? "" }
        XCTAssertEqual(paths, ["/v1/cabals/\(cabalID)/access-requests"])
    }

    func testAnAlreadyPendingRequestIsNotAMember() async {
        let transport = StubTransport(scripted: [Self.problem(409, "request_pending", "Your request is waiting.")])
        let entry = await api(transport).enterCabal(cabalID, submission: IdempotentSubmission())
        XCTAssertEqual(entry, .requestPending)
        XCTAssertFalse(entry.isMember)
    }

    func testAnExistingMemberIsReportedAsOne() async {
        let transport = StubTransport(scripted: [Self.problem(409, "already_member", "You're already in this cabal.")])
        let entry = await api(transport).enterCabal(cabalID, submission: IdempotentSubmission())
        XCTAssertEqual(entry, .alreadyMember)
    }

    func testAnyOtherRefusalIsSurfaced() async {
        let transport = StubTransport(scripted: [Self.problem(409, "cabal_banned", "This cabal was banned.")])
        let entry = await api(transport).enterCabal(cabalID, submission: IdempotentSubmission())
        guard case .refused(let error) = entry else { return XCTFail("expected a refusal, got \(entry)") }
        XCTAssertEqual(ToastCopy.message(for: error), "This cabal was banned.")
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

    func testOnlyAnExistingMemberIsAMember() {
        XCTAssertTrue(CabalEntry.alreadyMember.isMember)
        XCTAssertFalse(CabalEntry.requested.isMember)
        XCTAssertFalse(CabalEntry.requestPending.isMember)
    }

    func testTheJoinPreviewAsksToJoinFromTheFixtures() async {
        let model = JoinCabalModel.preview()
        model.code = "ABCD2345XY"
        await model.lookUp()
        XCTAssertEqual(model.actionTitle, "Ask to join")
        let joined = await model.submit()
        XCTAssertEqual(joined?.toast, CabalEntry.requestedToast)
    }

    private func api(_ transport: StubTransport) -> APIClient {
        APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
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
