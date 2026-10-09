import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class BlockedPeopleModelTests: XCTestCase {
    private func model(_ replies: StubTransport.Reply...) -> (BlockedPeopleModel, StubTransport) {
        let transport = StubTransport(scripted: replies)
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        return (BlockedPeopleModel(api: api), transport)
    }

    func testListsTheBlockedUsersNewestFirst() async {
        let (model, transport) = model(
            .json(
                .ok,
                #"{"users":[{"user_id":"u-1","handle":"maya","display_name":"Maya Angelou","photo_url":null},"#
                    + #"{"user_id":"u-2","handle":"","display_name":"","photo_url":null}]}"#))

        await model.load()

        guard case .loaded(let people) = model.state else { return XCTFail("want rows, got \(model.state)") }
        XCTAssertEqual(people.map(\.id), ["u-1", "u-2"])
        XCTAssertEqual(people.map(\.title), ["Maya Angelou", "Deleted account"])
        XCTAssertEqual(people.map(\.subtitle), ["@maya", nil])
        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.path), ["/v1/me/blocks"])
    }

    func testNoOneBlockedIsAnEmptyList() async {
        let (model, _) = model(.json(.ok, #"{"users":[]}"#))

        await model.load()

        guard case .loaded(let people) = model.state else { return XCTFail("want rows, got \(model.state)") }
        XCTAssertTrue(people.isEmpty)
    }

    func testAFailedFirstLoadIsTheFailedState() async {
        let (model, _) = model(.failure(URLError(.notConnectedToInternet)))

        await model.load()

        guard case .failed = model.state else { return XCTFail("want failed, got \(model.state)") }
    }
}
