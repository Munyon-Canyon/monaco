import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class CabalChatUnreadModelTests: XCTestCase {
    private static let cabalID = "cabal-1"
    private static let myCabalsPath = "/v1/me/cabals"

    func testTheUnreadCountLoadsAndRaisesTheDot() async throws {
        let (model, _, _) = try make([.json(.ok, Self.myCabals([(Self.cabalID, 3)]))])

        await model.load()

        XCTAssertEqual(model.unreadCount, 3)
        XCTAssertTrue(model.hasUnread)
    }

    func testAnotherCabalsRowIsIgnored() async throws {
        let (model, _, _) = try make([.json(.ok, Self.myCabals([("cabal-2", 5)]))])

        await model.load()

        XCTAssertEqual(model.unreadCount, 0)
        XCTAssertFalse(model.hasUnread)
    }

    func testAMissingRowReadsZero() async throws {
        let (model, _, _) = try make([.json(.ok, Self.myCabals([]))])

        await model.load()

        XCTAssertFalse(model.hasUnread)
    }

    func testMarkReadClearsTheCount() async throws {
        let (model, _, _) = try make([.json(.ok, Self.myCabals([(Self.cabalID, 2)]))])
        await model.load()

        model.markRead()

        XCTAssertEqual(model.unreadCount, 0)
        XCTAssertFalse(model.hasUnread)
    }

    func testAFailedReadClearsTheDot() async throws {
        let (model, _, _) = try make([
            .json(.ok, Self.myCabals([(Self.cabalID, 2)])), .failure(URLError(.notConnectedToInternet)),
        ])
        await model.load()
        XCTAssertTrue(model.hasUnread)

        await model.load()

        XCTAssertFalse(model.hasUnread)
    }

    func testAHintRefreshRaisesTheCount() async throws {
        let (model, _, hints) = try make([
            .json(.ok, Self.myCabals([(Self.cabalID, 0)])), .json(.ok, Self.myCabals([(Self.cabalID, 4)])),
        ])
        await model.load()
        model.setVisible(true)
        let observer = Task { await model.observe() }
        await hints.waitForSubscribers(2)

        await hints.send(.changed(.cabal(Self.cabalID), what: "chat", id: "1"))

        for _ in 0..<500 {
            if model.unreadCount == 4 { break }
            await Task.yield()
        }
        XCTAssertEqual(model.unreadCount, 4)
        observer.cancel()
    }

    private func make(
        _ replies: [StubTransport.Reply]
    ) throws -> (CabalChatUnreadModel, StubTransport, FakeHintStream) {
        let transport = StubTransport(routes: [Self.myCabalsPath: replies])
        let hints = FakeHintStream()
        let model = CabalChatUnreadModel(
            cabalID: Self.cabalID,
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            hints: hints)
        return (model, transport, hints)
    }

    private static func myCabals(_ rows: [(String, Int)]) -> String {
        let value = rows.map { id, unread in
            Components.Schemas.MyCabal(
                id: id, name: "QA pot", pictureUrl: nil,
                status: "active", role: "member", canVote: true, memberCount: 2,
                joinedAt: Date(timeIntervalSince1970: 0), pendingRequestCount: 0, unreadCount: unread)
        }
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return String(decoding: (try? encoder.encode(value)) ?? Data(), as: UTF8.self)
    }
}
