import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class ChatSeenModelTests: XCTestCase {
    private let cabalID = ChatFixtures.cabalID
    private let messageID = "01920000-0000-7000-8000-000000000007"

    private func model(_ transport: StubTransport) -> ChatSeenModel {
        ChatSeenModel(
            cabalID: cabalID,
            messageID: messageID,
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        )
    }

    func testLoadAsksForTheMessageAndListsTheViewers() async {
        let body =
            #"{"count":1,"members":[{"user_id":"u1","handle":"kai","display_name":"Kai","photo_url":null}]}"#
        let transport = StubTransport(.json(.ok, body))
        let model = model(transport)

        await model.load()

        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.path), ["/v1/cabals/\(cabalID)/chat/seen?message_id=\(messageID)"])
        guard case .loaded(let members) = model.state else {
            XCTFail("expected members, got \(model.state)")
            return
        }
        XCTAssertEqual(members.map(\.userId), ["u1"])
        XCTAssertEqual(members.map(ChatSeenCopy.name), ["Kai"])
        XCTAssertEqual(members.map(ChatSeenCopy.handle), ["@kai"])
    }

    func testAFailedLoadCanBeRetried() async {
        let body = #"{"count":0,"members":[]}"#
        let transport = StubTransport(scripted: [
            .failure(URLError(.notConnectedToInternet)), .json(.ok, body),
        ])
        let model = model(transport)

        await model.load()
        guard case .failed = model.state else {
            XCTFail("expected a failure, got \(model.state)")
            return
        }
        await model.load()

        XCTAssertEqual(model.state, .loaded([]))
    }

    func testAFormerMemberHasNoHandleAndAGenericName() {
        let member = Components.Schemas.ChatSeenMember(userId: "u2", handle: nil, displayName: " ", photoUrl: nil)

        XCTAssertEqual(ChatSeenCopy.name(member), "Former member")
        XCTAssertNil(ChatSeenCopy.handle(member))
    }
}
