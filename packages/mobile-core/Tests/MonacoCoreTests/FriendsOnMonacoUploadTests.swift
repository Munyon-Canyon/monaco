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
final class FriendsOnMonacoUploadTests: XCTestCase {
    private static let rawNumber = FriendsOnMonacoModelTests.rawNumber
    private static let page = FriendsOnMonacoModelTests.page
    private static let post = "/v1/me/contacts/match"
    private static let read = "/v1/me/contacts/matches"

    func testOpeningTwentyTimesInAnHourPostsOnce() async {
        let store = MemoryStore()
        let contacts = FakeContacts(access: .granted, numbers: [Self.rawNumber])
        let transport = StubTransport(.json(.ok, Self.page))
        for _ in 0..<20 {
            let model = makeModel(transport, contacts: contacts, userID: "user-1", store: store)
            await model.loadIfGranted()
            XCTAssertEqual(model.phase, .loaded)
            XCTAssertEqual(model.friends.map(\.handle), ["maya"])
        }
        let paths = await sentPaths(transport)
        XCTAssertEqual(paths.filter { $0 == Self.post }.count, 1)
        XCTAssertEqual(paths.filter { $0 == Self.read }.count, 20)
    }

    func testAnOldPostOrAChangedAddressBookPostsAgain() async {
        let start = Date()
        let store = MemoryStore()
        let contacts = FakeContacts(access: .granted, numbers: [])
        let transport = StubTransport(.json(.ok, Self.page))
        let window = FriendsOnMonacoModel.postedWindow
        let changed = [Self.rawNumber, "+442079460958"]
        for (offset, numbers) in [
            (0, [Self.rawNumber]), (window - 1, [Self.rawNumber]), (window, [Self.rawNumber]), (window + 1, changed),
        ] {
            contacts.numbers = numbers
            let model = makeModel(
                transport, contacts: contacts, userID: "user-1", store: store, now: start.addingTimeInterval(offset))
            await model.loadIfGranted()
        }
        let paths = await sentPaths(transport)
        XCTAssertEqual(paths, [Self.post, Self.read, Self.read, Self.post, Self.read, Self.post, Self.read])
    }

    func testAPostedRecordBelongsToOneAccount() async {
        let store = MemoryStore()
        let contacts = FakeContacts(access: .granted, numbers: [Self.rawNumber])
        let transport = StubTransport(.json(.ok, Self.page))
        for userID in ["user-1", "user-2", nil, nil] {
            let model = makeModel(transport, contacts: contacts, userID: userID, store: store)
            await model.loadIfGranted()
        }
        let paths = await sentPaths(transport)
        XCTAssertEqual(paths.filter { $0 == Self.post }.count, 4)
    }

    func testARateLimitedPostStillShowsTheStoredMatchesAndIsNotRemembered() async {
        let store = MemoryStore()
        let contacts = FakeContacts(access: .granted, numbers: [Self.rawNumber])
        let transport = StubTransport(scripted: [
            FriendsOnMonacoModelTests.problem(429, "rate_limited", "Slow down."), .json(.ok, Self.page),
            .json(.ok, Self.page), .json(.ok, Self.page),
        ])
        let model = makeModel(transport, contacts: contacts, userID: "user-1", store: store)
        await model.loadIfGranted()
        XCTAssertEqual(model.phase, .loaded)
        XCTAssertEqual(model.friends.map(\.handle), ["maya"])
        XCTAssertEqual(model.toast, "Slow down.")
        let next = makeModel(transport, contacts: contacts, userID: "user-1", store: store)
        await next.loadIfGranted()
        let paths = await sentPaths(transport)
        XCTAssertEqual(paths, [Self.post, Self.read, Self.post, Self.read])
    }

    func testACancelledCheckEndsIdleAndTheNextLoadUploadsAgain() async {
        let contacts = FakeContacts(access: .granted, numbers: [Self.rawNumber])
        let transport = StubTransport(scripted: [.hang, .json(.ok, Self.page), .json(.ok, Self.page)])
        let model = makeModel(transport, contacts: contacts, userID: "user-1")
        let check = Task { await model.loadIfGranted() }
        await transport.waitForRequest()
        check.cancel()
        await check.value
        XCTAssertEqual(model.phase, .idle)
        XCTAssertNil(model.toast)
        await model.loadIfGranted()
        XCTAssertEqual(model.phase, .loaded)
        XCTAssertEqual(model.friends.map(\.handle), ["maya"])
    }

    func testACheckCancelledAfterThePostDoesNotPostAgain() async {
        let contacts = FakeContacts(access: .granted, numbers: [Self.rawNumber])
        let transport = StubTransport(scripted: [.json(.ok, Self.page), .hang, .json(.ok, Self.page)])
        let model = makeModel(transport, contacts: contacts, userID: "user-1")
        let check = Task { await model.loadIfGranted() }
        await transport.waitForRequests(2)
        check.cancel()
        await check.value
        XCTAssertEqual(model.phase, .idle)
        await model.loadIfGranted()
        XCTAssertEqual(model.phase, .loaded)
        let paths = await sentPaths(transport)
        XCTAssertEqual(paths, [Self.post, Self.read, Self.read])
    }

    private func sentPaths(_ transport: StubTransport) async -> [String] {
        FriendsOnMonacoModelTests.paths(await transport.sent)
    }

    private func makeModel(
        _ transport: StubTransport, contacts: FakeContacts, userID: String? = nil,
        store: MemoryStore = MemoryStore(), now: Date = Date()
    ) -> FriendsOnMonacoModel {
        FriendsOnMonacoModel(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            contacts: contacts,
            defaultRegion: "US",
            userID: userID,
            store: store,
            now: { now }
        )
    }
}
