import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

private final class MemoryStore: KeyValueStoring {
    var values: [String: Any] = [:]

    func data(forKey key: String) -> Data? { values[key] as? Data }
    func bool(forKey key: String) -> Bool { values[key] as? Bool ?? false }
    func set(_ value: Any?, forKey key: String) { values[key] = value }
    func removeObject(forKey key: String) { values[key] = nil }
}

@MainActor
final class ReferralEntryModelTests: XCTestCase {
    private static let created =
        #"{"referrer":{"user_id":"01890a5d-ac96-774b-bcce-b302099a8058","display_name":"Kai","#
        + #""photo_url":null,"handle":"kaicenat"}}"#

    func testABareCodeAttachesAsManual() async throws {
        let (model, transport) = make([.json(.created, Self.created)])
        model.text = "  K7M4QX2P "

        let result = await model.submit()

        XCTAssertEqual(result, .attached(toast: "You joined from @kaicenat's invite."))
        let bodies = await transport.sentBodies
        let body = try JSONSerialization.jsonObject(with: try XCTUnwrap(bodies[0])) as? [String: String]
        XCTAssertEqual(body, ["code": "k7m4qx2p", "source": "manual"])
    }

    func testAFullLinkAttachesItsCode() async throws {
        let (model, transport) = make([.json(.created, Self.created)])
        model.text = "https://www.monacolabs.xyz/r/kaicenat"

        let result = await model.submit()

        XCTAssertEqual(result, .attached(toast: "You joined from @kaicenat's invite."))
        let bodies = await transport.sentBodies
        let body = try JSONSerialization.jsonObject(with: try XCTUnwrap(bodies[0])) as? [String: String]
        XCTAssertEqual(body?["code"], "kaicenat")
    }

    func testSomethingThatIsNeitherFailsWithoutACall() async {
        let (model, transport) = make([])
        model.text = "not a code!"

        let result = await model.submit()

        XCTAssertEqual(result, .failed(toast: "That code isn't valid."))
        let count = await transport.sent.count
        XCTAssertEqual(count, 0)
    }

    func testAnUnknownCodeFailsWithTheServerMessage() async throws {
        let problem = try StubTransport.Reply.problem(
            Components.Schemas.Problem(
                _type: .about_colon_blank, title: "Error", status: 404, code: .referralCodeUnknown,
                message: "That code isn't valid.", traceId: "4bf92f3577b34da6a3ce929d0e0e4736", retryable: false))
        let (model, _) = make([problem])
        model.text = "zzzzzzzz"

        let result = await model.submit()

        XCTAssertEqual(result, .failed(toast: "That code isn't valid."))
    }

    func testAnOfflineSubmitFailsAndCanBeRetried() async {
        let (model, _) = make([.failure(URLError(.notConnectedToInternet)), .json(.created, Self.created)])
        model.text = "k7m4qx2p"

        let offline = await model.submit()
        let retried = await model.submit()

        XCTAssertEqual(offline, .failed(toast: "You're offline. Try again."))
        XCTAssertEqual(retried, .attached(toast: "You joined from @kaicenat's invite."))
    }

    func testBlankTextCannotSubmit() async {
        let (model, transport) = make([])
        model.text = "   "

        let result = await model.submit()

        XCTAssertFalse(model.canSubmit)
        XCTAssertNil(result)
        let count = await transport.sent.count
        XCTAssertEqual(count, 0)
    }

    private func make(_ replies: [StubTransport.Reply]) -> (ReferralEntryModel, StubTransport) {
        let transport = StubTransport(scripted: replies)
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        let attacher = ReferralAttacher(
            api: api, store: MemoryStore(), now: { Date(timeIntervalSince1970: 1_800_000_000) })
        return (ReferralEntryModel(attacher: attacher, userID: "user-1"), transport)
    }
}
