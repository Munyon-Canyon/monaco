import Foundation
import HTTPTypes
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class CabalInviteAnswerTests: XCTestCase {
    private static let cabalID = "01890a5d-ac96-774b-bcce-b302099a8060"

    func testAcceptingAnInvitePostsTheApprovalOnceAndJoins() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.encode(Self.cabal(role: nil, mode: "request", request: ("i1", "invite")))),
            .json(.ok, #"{"id":"i1","direction":"invite","status":"approved"}"#),
        ])
        let model = makeModel(transport)
        await model.load()
        await model.accept()
        XCTAssertEqual(model.membershipChanges, 1)
        XCTAssertEqual(model.toast?.message, CabalEntry.joinedToast)
        XCTAssertEqual(model.toast?.isSuccess, true)
        let sent = await transport.sent
        XCTAssertEqual(sent.count, 2)
        XCTAssertEqual(sent[1].method, .post)
        XCTAssertEqual(sent[1].path, "/v1/cabals/\(Self.cabalID)/access-requests/i1/decision")
        let bodies = await transport.sentBodies
        let body = try XCTUnwrap(bodies[1])
        XCTAssertEqual(try JSONSerialization.jsonObject(with: body) as? [String: String], ["decision": "approve"])
    }

    func testDecliningAnInvitePostsADenialAndReloads() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.encode(Self.cabal(role: nil, mode: "request", request: ("i1", "invite")))),
            .json(.ok, #"{"id":"i1","direction":"invite","status":"denied"}"#),
            .json(
                .ok,
                try Self.encode(Self.cabal(role: nil, mode: "request", request: ("i1", "invite"), status: "denied"))),
        ])
        let model = makeModel(transport)
        await model.load()
        await model.decline()
        XCTAssertEqual(model.toast?.message, "Invite declined.")
        XCTAssertEqual(model.standing, .join)
        XCTAssertEqual(model.membershipChanges, 0)
        let bodies = await transport.sentBodies
        let body = try XCTUnwrap(bodies[1])
        XCTAssertEqual(try JSONSerialization.jsonObject(with: body) as? [String: String], ["decision": "deny"])
    }

    func testAFailedAcceptKeepsTheInviteAndSaysWhy() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.encode(Self.cabal(role: nil, mode: "request", request: ("i1", "invite")))),
            .failure(URLError(.notConnectedToInternet)),
        ])
        let model = makeModel(transport)
        await model.load()
        await model.accept()
        XCTAssertEqual(model.toast?.message, "You're offline. Try again.")
        XCTAssertEqual(model.standing, .invited(requestID: "i1"))
        XCTAssertEqual(model.membershipChanges, 0)
    }

    func testAFailedLoadFlagsTheFailureUntilALoadSucceeds() async throws {
        let transport = StubTransport(scripted: [
            .failure(URLError(.notConnectedToInternet)),
            .json(.ok, try Self.encode(Self.cabal(role: nil, mode: "request"))),
        ])
        let model = makeModel(transport)
        await model.load()
        XCTAssertTrue(model.loadFailed)
        XCTAssertEqual(model.standing, .hidden)
        await model.load()
        XCTAssertFalse(model.loadFailed)
        XCTAssertEqual(model.standing, .join)
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
}
