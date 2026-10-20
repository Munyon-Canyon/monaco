import Foundation
import HTTPTypes
import MonacoAPI
import MonacoCore
import MonacoTestClock
import MonacoTestSupport
import XCTest

@MainActor
final class FeedLikeTests: XCTestCase {
    private func liked(_ count: Int32, by me: Bool) -> Components.Schemas.FeedItem {
        var item = Components.Schemas.FeedItem.samples[0]
        item.likeCount = count
        item.likedByMe = me
        return item
    }

    func testLikingRaisesTheCountAndPutsTheLike() async throws {
        let item = liked(3, by: false)
        let transport = StubTransport(scripted: [.json(.ok, try Self.page([item])), .json(.noContent, "")])
        let model = makeModel(transport)
        await model.load()
        await model.toggleLike(item)
        XCTAssertEqual(model.items.first?.likedByMe, true)
        XCTAssertEqual(model.items.first?.likeCount, 4)
        let request = await transport.sent.last
        XCTAssertEqual(request?.method, .put)
        XCTAssertEqual(request?.path, "/v1/feed/likes/\(item.id)")
        let key = HTTPField.Name(IdempotentSubmission.keyHeader).flatMap { request?.headerFields[$0] }
        XCTAssertNotNil(key)
        XCTAssertEqual(model.failureTick, 0)
    }

    func testUnlikingLowersTheCountAndDeletesTheLike() async throws {
        let item = liked(3, by: true)
        let transport = StubTransport(scripted: [.json(.ok, try Self.page([item])), .json(.noContent, "")])
        let model = makeModel(transport)
        await model.load()
        await model.toggleLike(item)
        XCTAssertEqual(model.items.first?.likedByMe, false)
        XCTAssertEqual(model.items.first?.likeCount, 2)
        let request = await transport.sent.last
        XCTAssertEqual(request?.method, .delete)
        XCTAssertEqual(request?.path, "/v1/feed/likes/\(item.id)")
    }

    func testAFailedLikeRollsBackAndRaisesTheToast() async throws {
        let item = liked(3, by: false)
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.page([item])), .failure(URLError(.notConnectedToInternet)),
        ])
        let model = makeModel(transport)
        await model.load()
        await model.toggleLike(item)
        XCTAssertEqual(model.items.first?.likedByMe, false)
        XCTAssertEqual(model.items.first?.likeCount, 3)
        XCTAssertEqual(model.failureTick, 1)
        XCTAssertEqual(model.lastError.map(ToastCopy.message(for:)), "You're offline. Try again.")
    }

    func testAFailedUnlikeRollsBackToLiked() async throws {
        let item = liked(1, by: true)
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.page([item])), .failure(URLError(.notConnectedToInternet)),
        ])
        let model = makeModel(transport)
        await model.load()
        await model.toggleLike(item)
        XCTAssertEqual(model.items.first?.likedByMe, true)
        XCTAssertEqual(model.items.first?.likeCount, 1)
        XCTAssertEqual(model.failureTick, 1)
    }

    func testTheHeartFillsBeforeTheServerAnswersAndASecondTapWaits() async throws {
        let item = liked(0, by: false)
        let transport = StubTransport(scripted: [.json(.ok, try Self.page([item])), .gate])
        let model = makeModel(transport)
        await model.load()
        let first = Task { await model.toggleLike(item) }
        let asked = await waitUntil { await transport.sent.count == 2 }
        XCTAssertTrue(asked)
        XCTAssertEqual(model.items.first?.likedByMe, true)
        XCTAssertEqual(model.items.first?.likeCount, 1)
        await model.toggleLike(item)
        let sent = await transport.sent.count
        XCTAssertEqual(sent, 2)
        await transport.releaseGate(.json(.noContent, ""))
        await first.value
        XCTAssertEqual(model.items.first?.likedByMe, true)
    }

    func testTheLikeCountNeverGoesBelowZero() async throws {
        let item = liked(0, by: true)
        let transport = StubTransport(scripted: [.json(.ok, try Self.page([item])), .json(.noContent, "")])
        let model = makeModel(transport)
        await model.load()
        await model.toggleLike(item)
        XCTAssertEqual(model.items.first?.likeCount, 0)
    }

    private func makeModel(_ transport: StubTransport) -> FeedModel {
        FeedModel(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            viewerID: nil, hints: FakeHintStream(), clock: TestClock())
    }

    private func waitUntil(_ predicate: @escaping () async -> Bool) async -> Bool {
        for _ in 0..<500 {
            if await predicate() { return true }
            await Task.yield()
        }
        return await predicate()
    }

    private static func page(_ items: [Components.Schemas.FeedItem]) throws -> String {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return String(
            decoding: try encoder.encode(Components.Schemas.FeedPage(items: items, nextCursor: nil)), as: UTF8.self)
    }
}
