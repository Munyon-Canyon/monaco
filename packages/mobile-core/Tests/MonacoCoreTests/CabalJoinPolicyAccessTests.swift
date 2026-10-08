import Foundation
import HTTPTypes
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class CabalJoinPolicyAccessTests: XCTestCase {
    private static let cabalID = "01890a5d-ac96-774b-bcce-b302099a8060"

    func testAnOpenCabalReadsAsOpenAndOffersAnImmediateJoin() async throws {
        let model = makeModel(StubTransport(.json(.ok, try Self.encode(Self.cabal(role: nil, mode: "open")))))
        await model.load()
        XCTAssertEqual(model.standing, .join)
        XCTAssertEqual(model.joinPolicy, .open)
        XCTAssertEqual(model.joinPolicy.prospectNote, "Open: anyone can join.")
    }

    func testJoiningAnOpenCabalMakesTheCallerAMemberWithoutARequest() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.encode(Self.cabal(role: nil, mode: "open"))),
            .json(.ok, try Self.encode(Self.cabal(role: "member", mode: "open"))),
            .json(.ok, try Self.encode(Self.cabal(role: "member", mode: "open"))),
        ])
        let model = makeModel(transport)
        await model.load()
        await model.enter()
        XCTAssertEqual(model.toast?.message, CabalEntry.joinedToast)
        XCTAssertEqual(model.toast?.isSuccess, true)
        XCTAssertEqual(model.membershipChanges, 1)
        XCTAssertEqual(model.standing, .hidden)
        let paths = await transport.sent.map { $0.path ?? "" }
        XCTAssertEqual(paths[1], "/v1/cabals/\(Self.cabalID)/members")
        XCTAssertFalse(paths.contains { $0.hasSuffix("/access-requests") })
    }

    func testADeniedRequestShowsDeclinedAndAskingAgainSendsANewRequest() async throws {
        let denied = Self.cabal(role: nil, mode: "request", request: ("r0", "request"), status: "denied")
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.encode(denied)),
            .json(.created, #"{"id":"r1","direction":"request","status":"pending"}"#),
            .json(.ok, try Self.encode(Self.cabal(role: nil, mode: "request", request: ("r1", "request")))),
        ])
        let model = makeModel(transport)
        await model.load()
        XCTAssertEqual(model.standing, .declined)
        await model.enter()
        XCTAssertEqual(model.toast?.message, CabalEntry.requestedToast)
        XCTAssertEqual(model.standing, .requested(requestID: "r1"))
        let paths = await transport.sent.map { $0.path ?? "" }
        XCTAssertEqual(paths[1], "/v1/cabals/\(Self.cabalID)/access-requests")
    }

    func testADeniedInviteIsNotADeclinedRequest() async throws {
        let cabal = Self.cabal(role: nil, mode: "request", request: ("i1", "invite"), status: "denied")
        let model = makeModel(StubTransport(.json(.ok, try Self.encode(cabal))))
        await model.load()
        XCTAssertEqual(model.standing, .join)
    }

    func testAskingAgainOnACabalThatTurnedOpenJoinsInstead() async throws {
        let denied = Self.cabal(role: nil, mode: "request", request: ("r0", "request"), status: "denied")
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.encode(denied)),
            Self.problem(409, "request_not_needed", "This cabal is open."),
            .json(.ok, try Self.encode(Self.cabal(role: "member", mode: "open"))),
            .json(.ok, try Self.encode(Self.cabal(role: "member", mode: "open"))),
        ])
        let model = makeModel(transport)
        await model.load()
        await model.enter()
        XCTAssertEqual(model.toast?.message, CabalEntry.joinedToast)
        XCTAssertEqual(model.membershipChanges, 1)
    }

    private func makeModel(_ transport: StubTransport) -> CabalAccessModel {
        CabalAccessModel(
            cabalID: Self.cabalID,
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            hints: FakeHintStream()
        )
    }

    private static func cabal(
        role: String?, mode: String, request: (String, String)? = nil, status: String = "pending"
    ) -> Components.Schemas.Cabal {
        var cabal = Components.Schemas.Cabal.sample(role: role)
        cabal.rules.joinMode = mode
        cabal.myAccessRequest = request.map { .init(id: $0.0, direction: $0.1, status: status) }
        return cabal
    }

    private static func encode(_ value: some Encodable) throws -> String {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return String(decoding: try encoder.encode(value), as: UTF8.self)
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
