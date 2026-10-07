import Foundation
import HTTPTypes
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class JoinCabalModelTests: XCTestCase {
    private let cabalID = "01890a5d-ac96-774b-bcce-b302099a8060"

    func testAPartialCodeLooksNothingUp() async {
        let transport = StubTransport(scripted: [])
        let model = makeModel(transport)
        model.code = "ABCD23"
        await model.lookUp()
        XCTAssertEqual(model.lookup, .none)
        XCTAssertNil(model.message)
        XCTAssertFalse(model.canSubmit)
        let sent = await transport.sent
        XCTAssertTrue(sent.isEmpty)
    }

    func testACabalIDIsNotACode() async {
        let transport = StubTransport(scripted: [])
        let model = makeModel(transport)
        model.code = cabalID
        await model.lookUp()
        XCTAssertEqual(model.message, JoinCabalModel.malformedMessage)
        XCTAssertFalse(model.canSubmit)
        let sent = await transport.sent
        XCTAssertTrue(sent.isEmpty)
    }

    func testAPastedCodeResolvesToTheCabalByCode() async throws {
        let transport = StubTransport(.json(.ok, try Self.preview("request")))
        let model = makeModel(transport)
        model.code = " abcd2345xy "
        await model.lookUp()
        XCTAssertEqual(model.preview?.name, "QA pot")
        XCTAssertEqual(model.actionTitle, "Ask to join")
        let paths = await transport.sent.map { $0.path ?? "" }
        XCTAssertEqual(paths, ["/v1/invite-codes/ABCD2345XY"])
    }

    func testAnUnknownCodeSaysSoUnderTheField() async {
        let model = makeModel(StubTransport(scripted: [Self.problem(404, "cabal_not_found", "No such cabal.")]))
        model.code = "ABCD2345XY"
        await model.lookUp()
        XCTAssertEqual(model.message, "No cabal with that invite code")
        XCTAssertFalse(model.canSubmit)
    }

    func testEditingTheCodeDropsTheOldAnswer() async throws {
        let model = makeModel(StubTransport(scripted: [Self.problem(404, "cabal_not_found", "No such cabal.")]))
        model.code = "ABCD2345XY"
        await model.lookUp()
        model.code = "ABCD2345XZ"
        XCTAssertNil(model.message)
        XCTAssertTrue(model.canSubmit)
    }

    func testAValidCodeFilesAPendingRequestAndDoesNotJoin() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.preview("request")),
            .json(.created, #"{"id":"r","direction":"request","status":"pending"}"#),
        ])
        let model = makeModel(transport)
        model.code = "ABCD2345XY"
        let joined = await model.submit()
        XCTAssertEqual(
            joined, JoinedCabal(cabalID: cabalID, toast: "Request sent. You'll be in once the creator says yes."))
        let sent = await transport.sent
        XCTAssertEqual(
            sent.map { $0.path ?? "" },
            ["/v1/invite-codes/ABCD2345XY", "/v1/cabals/\(cabalID)/access-requests"]
        )
        XCTAssertNotNil(sent[1].headerFields[try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))])
        XCTAssertFalse(CabalEntry.requested.isMember)
    }

    func testAlreadyAMemberOpensTheCabalWithoutAToast() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.preview("request")),
            Self.problem(409, "already_member", "You're already in this cabal."),
        ])
        let model = makeModel(transport)
        model.code = "ABCD2345XY"
        let joined = await model.submit()
        XCTAssertEqual(joined, JoinedCabal(cabalID: cabalID, toast: nil))
    }

    func testAPendingRequestShowsRequestSent() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.preview("request")),
            Self.problem(409, "request_pending", "Your request is waiting."),
        ])
        let model = makeModel(transport)
        model.code = "ABCD2345XY"
        let joined = await model.submit()
        XCTAssertNil(joined)
        XCTAssertEqual(model.actionTitle, "Request sent")
        XCTAssertFalse(model.canSubmit)
    }

    func testAnyOtherRefusalToastsTheServerMessage() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.preview("request")),
            Self.problem(409, "cabal_banned", "This cabal was banned."),
        ])
        let model = makeModel(transport)
        model.code = "ABCD2345XY"
        let joined = await model.submit()
        XCTAssertNil(joined)
        XCTAssertEqual(model.toast?.message, "This cabal was banned.")
    }

    func testARetryAfterADroppedConnectionReusesTheKey() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.preview("request")),
            .failure(URLError(.networkConnectionLost)),
            .json(.created, #"{"id":"r","direction":"request","status":"pending"}"#),
        ])
        let model = makeModel(transport)
        model.code = "ABCD2345XY"
        let first = await model.submit()
        XCTAssertNil(first)
        XCTAssertEqual(model.toast?.message, "You're offline. Try again.")
        let second = await model.submit()
        XCTAssertEqual(second?.cabalID, cabalID)
        let sent = await transport.sent
        let name = try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
        XCTAssertEqual(sent[1].headerFields[name], sent[2].headerFields[name])
    }

    private func makeModel(_ transport: StubTransport) -> JoinCabalModel {
        JoinCabalModel(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        )
    }

    private static func preview(_ joinMode: String) throws -> String {
        let preview = Components.Schemas.CabalPreview.sample(joinMode: joinMode)
        return String(decoding: try JSONEncoder().encode(preview), as: UTF8.self)
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
