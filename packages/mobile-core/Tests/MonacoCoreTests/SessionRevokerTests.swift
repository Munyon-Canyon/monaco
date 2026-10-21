import Foundation
import HTTPTypes
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

final class SessionRevokerTests: XCTestCase {
    private func revoker(_ transport: StubTransport) -> SessionRevoker {
        SessionRevoker { token in
            APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: token), transport: transport)
        }
    }

    func testItAsksTheServerToEndTheSessionOfTheTokenItWasGiven() async throws {
        let transport = StubTransport(status: .noContent, contentType: "application/json", body: Data())

        let revoked = await revoker(transport).revoke(token: "copied")

        XCTAssertTrue(revoked)
        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.method), [.post])
        XCTAssertEqual(sent.map(\.path), ["/v1/me/sessions/revoke"])
        XCTAssertEqual(sent.first?.headerFields[.authorization], "Bearer copied")
        XCTAssertNotNil(sent.first?.headerFields[try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))])
        let bodies = await transport.sentBodies
        let body = try XCTUnwrap(bodies.first ?? nil)
        let json = try XCTUnwrap(JSONSerialization.jsonObject(with: body) as? [String: String])
        XCTAssertEqual(json, ["scope": "this_session"])
    }

    func testAnUnreachableServerIsNotAnError() async {
        let transport = StubTransport(.failure(URLError(.notConnectedToInternet)))

        let revoked = await revoker(transport).revoke(token: "copied")

        XCTAssertFalse(revoked)
    }

    func testAnAlreadyRefusedTokenIsNotRetriedWithAnotherOne() async {
        let transport = StubTransport(status: .unauthorized, contentType: "application/json", body: Data("{}".utf8))

        let revoked = await revoker(transport).revoke(token: "copied")

        XCTAssertFalse(revoked)
        let sent = await transport.sent
        XCTAssertEqual(sent.count, 1)
    }

    func testAServerThatNeverAnswersDoesNotHoldSignOut() async {
        let transport = StubTransport(.hang)

        let revoked = await revoker(transport).revoke(token: "copied", within: .milliseconds(50))

        XCTAssertFalse(revoked)
    }
}
