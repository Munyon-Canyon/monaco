import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class CabalChatPreviewModelTests: XCTestCase {
    private static let cabalID = "cabal-1"
    private static let messagesPath = "/v1/cabals/cabal-1/messages?limit=1"
    private static let myCabalsPath = "/v1/me/cabals"

    func testTheNewestMessageAndTheUnreadCountLoadTogether() async throws {
        let (model, transport, _) = try make(
            messages: [.json(.ok, Self.page([Self.message()]))],
            myCabals: [.json(.ok, try Self.myCabals(unread: 3))])

        await model.load()

        guard case .loaded(let message) = model.phase else { return XCTFail("want a loaded message") }
        XCTAssertEqual(message.author, "Alex")
        XCTAssertEqual(message.body, "gm")
        XCTAssertEqual(model.unreadCount, 3)
        let paths = await transport.sent.map { $0.path ?? "?" }
        XCTAssertEqual(Set(paths), [Self.messagesPath, Self.myCabalsPath])
    }

    func testAChannelWithNoLiveMessageIsEmpty() async throws {
        let (model, _, _) = try make(
            messages: [.json(.ok, Self.page([]))], myCabals: [.json(.ok, try Self.myCabals(unread: 0))])

        await model.load()

        XCTAssertEqual(model.phase, .empty)
    }

    func testAFailedUnreadReadStillShowsTheMessage() async throws {
        let (model, _, _) = try make(
            messages: [.json(.ok, Self.page([Self.message()]))],
            myCabals: [.failure(URLError(.notConnectedToInternet))])

        await model.load()

        guard case .loaded = model.phase else { return XCTFail("want a loaded message") }
        XCTAssertEqual(model.unreadCount, 0)
    }

    func testAFailedFirstReadFailsAndALaterFailureKeepsTheMessage() async throws {
        let (model, _, _) = try make(
            messages: [
                .failure(URLError(.notConnectedToInternet)), .json(.ok, Self.page([Self.message()])),
                .failure(URLError(.notConnectedToInternet)),
            ],
            myCabals: Array(repeating: .json(.ok, try Self.myCabals(unread: 1)), count: 3))

        await model.load()
        XCTAssertEqual(model.phase, .failed)
        await model.load()
        guard case .loaded = model.phase else { return XCTFail("want a loaded message") }
        await model.load()
        guard case .loaded = model.phase else { return XCTFail("a later failure must keep the message") }
    }

    func testAHintReloadsTheMessage() async throws {
        let (model, transport, hints) = try make(
            messages: [.json(.ok, Self.page([Self.message()])), .json(.ok, Self.page([Self.message()]))],
            myCabals: Array(repeating: .json(.ok, try Self.myCabals(unread: 1)), count: 2))
        model.setVisible(true)
        let observer = Task { await model.observe() }
        await hints.waitForSubscribers(2)

        await hints.send(.changed(.cabal(Self.cabalID), what: "chat", id: "1"))

        for _ in 0..<500 {
            if await transport.sent.count >= 2 { break }
            await Task.yield()
        }
        let sent = await transport.sent.count
        XCTAssertGreaterThanOrEqual(sent, 2)
        observer.cancel()
    }

    private func make(
        messages: [StubTransport.Reply], myCabals: [StubTransport.Reply]
    ) throws -> (CabalChatPreviewModel, StubTransport, FakeHintStream) {
        let transport = StubTransport(routes: [Self.messagesPath: messages, Self.myCabalsPath: myCabals])
        let hints = FakeHintStream()
        let model = CabalChatPreviewModel(
            cabalID: Self.cabalID,
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            hints: hints)
        return (model, transport, hints)
    }

    private static func message() -> Components.Schemas.ChatMessage {
        Components.Schemas.ChatMessage(
            id: "m-1", author: .init(id: "u-1", handle: "alex", displayName: "Alex", photoUrl: nil), body: "gm",
            createdAt: Date(timeIntervalSince1970: 1_790_000_000), alsoInChannel: false, replyCount: 0, deleted: false)
    }

    private static func page(_ messages: [Components.Schemas.ChatMessage]) -> String {
        encode(Components.Schemas.ChatChannelPage(messages: messages))
    }

    private static func myCabals(unread: Int) throws -> String {
        encode([
            Components.Schemas.MyCabal(
                id: cabalID, name: "QA pot", pictureUrl: nil, role: "member", canVote: true, memberCount: 2,
                joinedAt: Date(timeIntervalSince1970: 0), pendingRequestCount: 0, unreadCount: unread)
        ])
    }

    private static func encode(_ value: some Encodable) -> String {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return String(decoding: (try? encoder.encode(value)) ?? Data(), as: UTF8.self)
    }
}
