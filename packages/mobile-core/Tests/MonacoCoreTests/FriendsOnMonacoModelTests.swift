import Foundation
import HTTPTypes
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class FriendsOnMonacoModelTests: XCTestCase {
    private static let rawNumber = "+14155550123"
    private static let contactName = "Ada Lovelace"
    private static let friendID = Components.Schemas.ContactMatch.sample.userId

    func testUploadPostsHashesOnlyThenLoadsMatches() async throws {
        let contacts = FakeContacts(access: .granted, numbers: [Self.rawNumber, Self.contactName, "4155550123"])
        let transport = StubTransport(scripted: [.json(.ok, Self.page), .json(.ok, Self.page)])
        let model = makeModel(transport, contacts: contacts, chunkSize: 2000)
        await model.loadIfGranted()
        XCTAssertEqual(model.phase, .loaded)
        XCTAssertEqual(model.friends.map(\.handle), ["maya"])
        let sent = await transport.sent
        XCTAssertEqual(Self.paths(sent), ["/v1/me/contacts/match", "/v1/me/contacts/matches"])
        await assertNoContactSecrets(transport)
    }

    func testEachChunkHasItsOwnIdempotencyKey() async throws {
        let contacts = FakeContacts(
            access: .granted, numbers: ["+14155550123", "+442079460958", "call me"])
        let transport = StubTransport(scripted: [
            .json(.ok, Self.emptyPage), .json(.ok, Self.emptyPage), .json(.ok, Self.emptyPage),
        ])
        let model = makeModel(transport, contacts: contacts, chunkSize: 1)
        await model.loadIfGranted()
        XCTAssertEqual(model.phase, .empty)
        let sent = await transport.sent
        XCTAssertEqual(
            Self.paths(sent), ["/v1/me/contacts/match", "/v1/me/contacts/match", "/v1/me/contacts/matches"])
        let name = try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
        let keys = sent.prefix(2).map { $0.headerFields[name] }
        XCTAssertNotNil(keys[0])
        XCTAssertNotNil(keys[1])
        XCTAssertNotEqual(keys[0], keys[1])
        await assertNoContactSecrets(transport)
    }

    func testDeniedAccessSendsNothing() async {
        let contacts = FakeContacts(access: .notDetermined, requestResult: .denied, numbers: [Self.rawNumber])
        let transport = StubTransport(scripted: [])
        let model = makeModel(transport, contacts: contacts, chunkSize: 2000)
        await model.findFriends()
        XCTAssertEqual(model.access, .denied)
        XCTAssertEqual(model.phase, .idle)
        let sent = await transport.sent
        XCTAssertTrue(sent.isEmpty)
    }

    func testAFailedLoadCanBeRetried() async {
        let contacts = FakeContacts(access: .granted, numbers: [])
        let transport = StubTransport(scripted: [
            Self.problem(500, "internal", "Something went wrong."),
            .json(.ok, Self.emptyPage),
        ])
        let model = makeModel(transport, contacts: contacts, chunkSize: 2000)
        await model.loadIfGranted()
        XCTAssertEqual(model.phase, .failed)
        XCTAssertNil(model.toast)
        await model.retry()
        XCTAssertEqual(model.phase, .empty)
    }

    func testFollowUsesThePhoneSource() async throws {
        let contacts = FakeContacts(access: .granted, numbers: [])
        let transport = StubTransport(scripted: [
            .json(.ok, Self.page),
            .json(.ok, #"{"following":true}"#),
        ])
        let model = makeModel(transport, contacts: contacts, chunkSize: 2000)
        await model.loadIfGranted()
        await model.follow(Self.friendID)
        XCTAssertEqual(model.friends.first?.followedByMe, true)
        let sent = await transport.sent
        XCTAssertEqual(sent.last?.path, "/v1/users/\(Self.friendID)/follow")
        let bodies = await transport.sentBodies
        let body = try XCTUnwrap(bodies.last ?? nil)
        XCTAssertEqual(try JSONSerialization.jsonObject(with: body) as? [String: String], ["source": "phone"])
        await assertNoContactSecrets(transport)
    }

    func testARateLimitKeepsTheListAndToastsTheProblemMessage() async throws {
        let contacts = FakeContacts(access: .granted, numbers: [])
        let transport = StubTransport(scripted: [
            .json(.ok, Self.page),
            Self.problem(429, "rate_limited", "Slow down."),
        ])
        let model = makeModel(transport, contacts: contacts, chunkSize: 2000)
        await model.loadIfGranted()
        await model.follow(Self.friendID)
        XCTAssertEqual(model.phase, .loaded)
        XCTAssertEqual(model.friends.first?.followedByMe, false)
        XCTAssertEqual(model.toast, "Slow down.")
        XCTAssertEqual(model.toastTick, 1)
    }

    func testAnUploadRateLimitToastsTheProblemMessage() async {
        let contacts = FakeContacts(access: .granted, numbers: [Self.rawNumber])
        let transport = StubTransport(scripted: [Self.problem(429, "rate_limited", "Slow down.")])
        let model = makeModel(transport, contacts: contacts, chunkSize: 2000)
        await model.loadIfGranted()
        XCTAssertEqual(model.phase, .failed)
        XCTAssertTrue(model.friends.isEmpty)
        XCTAssertEqual(model.toast, "Slow down.")
    }

    private static func paths(_ sent: [HTTPRequest]) -> [String] {
        sent.map { request in
            let path = request.path ?? ""
            guard let query = path.firstIndex(of: "?") else { return path }
            return String(path[..<query])
        }
    }

    private func makeModel(
        _ transport: StubTransport, contacts: FakeContacts, chunkSize: Int
    ) -> FriendsOnMonacoModel {
        FriendsOnMonacoModel(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            contacts: contacts,
            defaultRegion: "US",
            chunkSize: chunkSize
        )
    }

    private func assertNoContactSecrets(_ transport: StubTransport) async {
        let bodies = await transport.sentBodies
        let text = bodies.map { body -> String in
            guard let body else { return "" }
            return String(decoding: body, as: UTF8.self)
        }.joined(separator: "\n")
        XCTAssertFalse(text.contains(Self.rawNumber))
        XCTAssertFalse(text.contains("4155550123"))
        XCTAssertFalse(text.contains(Self.contactName))
        XCTAssertFalse(text.contains("Ada"))
    }

    private static let page =
        #"{"items":[{"user_id":"\#(friendID)","handle":"maya","display_name":"Maya","photo_url":null,"followed_by_me":false}],"next_cursor":null}"#

    private static let emptyPage = #"{"items":[],"next_cursor":null}"#

    private static func problem(_ status: Int, _ code: String, _ message: String) -> StubTransport.Reply {
        let body =
            #"{"type":"about:blank","title":"Error","status":\#(status),"code":"\#(code)","message":"\#(message)","#
            + #""trace_id":"00000000000000000000000000000000","retryable":false}"#
        return .response(
            status: .init(code: status), contentType: "application/problem+json", body: Data(body.utf8)
        )
    }
}

@MainActor
private final class FakeContacts: ContactsSource {
    var access: ContactsAccess
    var requestResult: ContactsAccess
    var numbers: [String]

    init(access: ContactsAccess, requestResult: ContactsAccess? = nil, numbers: [String]) {
        self.access = access
        self.requestResult = requestResult ?? access
        self.numbers = numbers
    }

    func currentAccess() -> ContactsAccess { access }

    func requestAccess() async -> ContactsAccess {
        access = requestResult
        return access
    }

    func phoneNumbers() throws -> [String] { numbers }
}
