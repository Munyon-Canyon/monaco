import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestClock
import MonacoTestSupport
import XCTest

@MainActor
final class FeedModelRefreshTests: XCTestCase {
    private let samples = Components.Schemas.FeedItem.samples

    func testAFeedHintFetchesPageOneOnce() async throws {
        let (model, transport, hints) = try await observed([
            .json(.ok, try Self.page(Array(samples.suffix(2)))),
            .json(.ok, try Self.page(samples)),
        ])

        await hints.send(.changed(.global, what: "feed", id: "1"))

        let merged = await waitUntil { model.items.count == self.samples.count }
        XCTAssertTrue(merged)
        let paths = await transport.sent.map(\.path)
        XCTAssertEqual(paths.count, 2)
        XCTAssertFalse(paths.last??.contains("cursor=") ?? true)
    }

    func testAHintForAnotherWhatFetchesNothing() async throws {
        let (_, transport, hints) = try await observed([.json(.ok, try Self.page(samples))])

        await hints.send(.changed(.global, what: "prices_updated", id: "1"))

        for _ in 0..<50 { await Task.yield() }
        let sent = await transport.sent.count
        XCTAssertEqual(sent, 1)
    }

    func testAResyncMergesThreeNewItemsAboveThirtyKnownAndKeepsAll() async throws {
        let known = Self.items(prefix: "known", count: 30)
        let fresh = Self.items(prefix: "fresh", count: 3)
        var recounted = Array(known.prefix(27))
        recounted[0].commentCount = 1
        let (model, transport, hints) = try await observed([
            .json(.ok, try Self.page(known, next: "c2")),
            .json(.ok, try Self.page(fresh + recounted, next: "c2b")),
        ])

        await hints.send(.resync)

        let merged = await waitUntil { model.items.count == 33 }
        XCTAssertTrue(merged, "\(model.items.count)")
        XCTAssertEqual(model.items.map(\.id), (fresh + known).map(\.id))
        XCTAssertEqual(model.items[3].commentCount, 1)
        let sent = await transport.sent.count
        XCTAssertEqual(sent, 2)
    }

    func testAHintWhileHiddenRefreshesOnceWhenTheTabComesBack() async throws {
        let (model, transport, hints) = try await observed([
            .json(.ok, try Self.page(Array(samples.suffix(2)))),
            .json(.ok, try Self.page(samples)),
        ])
        model.setVisible(false)

        await hints.send(.changed(.global, what: "feed", id: "1"))
        await hints.send(.changed(.global, what: "feed", id: "2"))

        for _ in 0..<50 { await Task.yield() }
        let whileHidden = await transport.sent.count
        XCTAssertEqual(whileHidden, 1)
        model.setVisible(true)
        let merged = await waitUntil { model.items.count == self.samples.count }
        XCTAssertTrue(merged)
        for _ in 0..<50 { await Task.yield() }
        let sent = await transport.sent.count
        XCTAssertEqual(sent, 2)
    }

    private func observed(
        _ script: [StubTransport.Reply]
    ) async throws -> (FeedModel, StubTransport, FakeHintStream) {
        let transport = StubTransport(scripted: script)
        let hints = FakeHintStream()
        let model = FeedModel(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            viewerID: nil,
            hints: hints,
            clock: TestClock()
        )
        await model.load()
        let observer = Task { await model.observe() }
        addTeardownBlock { observer.cancel() }
        let subscribed = await waitUntil { await hints.subscriberCount == 1 }
        XCTAssertTrue(subscribed)
        return (model, transport, hints)
    }

    private func waitUntil(_ predicate: @escaping () async -> Bool) async -> Bool {
        let deadline = ContinuousClock.now.advanced(by: .seconds(5))
        while ContinuousClock.now < deadline {
            if await predicate() { return true }
            await Task.yield()
        }
        return await predicate()
    }

    private static func items(prefix: String, count: Int) -> [Components.Schemas.FeedItem] {
        (0..<count).map { index in
            var item = Components.Schemas.FeedItem.samples[0]
            item.id = "\(prefix)-\(index)"
            item.commentCount = 0
            return item
        }
    }

    private static func page(_ items: [Components.Schemas.FeedItem], next: String? = nil) throws -> String {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        let page = Components.Schemas.FeedPage(items: items, nextCursor: next)
        return String(decoding: try encoder.encode(page), as: UTF8.self)
    }
}
