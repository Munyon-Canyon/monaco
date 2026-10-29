import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestClock
import MonacoTestSupport
import XCTest

@MainActor
final class FeedModelQuietTests: XCTestCase {
    private let now = Date(timeIntervalSince1970: 1_800_000_000)
    private let day: TimeInterval = 24 * 60 * 60

    func testAnEmptyPageIsAQuietWeekAndAsksForOthersOnce() async throws {
        let (model, transport) = try await loaded([try Self.page([]), try Self.page(others)])

        XCTAssertTrue(model.isQuietWeek)
        XCTAssertEqual(model.fromEveryone.map(\.id), others.map(\.id))
        let paths = await transport.sent.compactMap(\.path)
        XCTAssertEqual(paths.count, 2)
        XCTAssertEqual(paths.filter { $0.contains("scope=others") }.count, 1)
    }

    func testANewestItemEightDaysOldIsAQuietWeek() async throws {
        let (model, _) = try await loaded([try Self.page([item("old", daysAgo: 8)]), try Self.page(others)])

        XCTAssertTrue(model.isQuietWeek)
        XCTAssertEqual(model.items.map(\.id), ["old"])
        XCTAssertEqual(model.fromEveryone.map(\.id), others.map(\.id))
    }

    func testANewestItemSixDaysOldIsNotAQuietWeek() async throws {
        let (model, transport) = try await loaded([try Self.page([item("fresh", daysAgo: 6)])])

        XCTAssertFalse(model.isQuietWeek)
        XCTAssertTrue(model.fromEveryone.isEmpty)
        let sent = await transport.sent.count
        XCTAssertEqual(sent, 1)
    }

    func testTheNewestItemDecidesNotTheFirstOne() async throws {
        let (model, _) = try await loaded([try Self.page([item("old", daysAgo: 9), item("fresh", daysAgo: 1)])])

        XCTAssertFalse(model.isQuietWeek)
    }

    func testYourCabalsIsNeverAQuietWeek() async throws {
        let (model, transport) = try await loaded([
            try Self.page([item("old", daysAgo: 9)]), try Self.page(others), try Self.page([item("old", daysAgo: 9)]),
        ])

        await model.select(.mine)

        XCTAssertFalse(model.isQuietWeek)
        XCTAssertTrue(model.fromEveryone.isEmpty)
        let paths = await transport.sent.compactMap(\.path)
        XCTAssertEqual(paths.count, 3)
        XCTAssertEqual(paths.filter { $0.contains("scope=others") }.count, 1)
    }

    func testTheOthersRequestAsksForFour() async throws {
        let (_, transport) = try await loaded([try Self.page([]), try Self.page(others)])

        let paths = await transport.sent.compactMap(\.path)
        let query = try XCTUnwrap(paths.first { $0.contains("scope=others") })
        XCTAssertTrue(query.contains("limit=4"), query)
    }

    func testAFailedOthersRequestLeavesTheListEmptyButTheWeekQuiet() async throws {
        let (model, _) = try await loaded([try Self.page([]), .failure(URLError(.notConnectedToInternet))])

        XCTAssertTrue(model.isQuietWeek)
        XCTAssertTrue(model.fromEveryone.isEmpty)
    }

    func testAHintRefreshesBothLists() async throws {
        let hints = FakeHintStream()
        let (model, transport) = try await loaded(
            [
                try Self.page([item("old", daysAgo: 9)]), try Self.page(Array(others.prefix(1))),
                try Self.page([item("fresh", daysAgo: 10), item("old", daysAgo: 9)]), try Self.page(others),
            ], hints: hints)
        let observer = Task { await model.observe() }
        addTeardownBlock { observer.cancel() }
        let subscribed = await waitUntil { await hints.subscriberCount == 1 }
        XCTAssertTrue(subscribed)

        await hints.send(.changed(.global, what: "feed", id: "1"))

        let refreshed = await waitUntil { model.fromEveryone.count == self.others.count }
        XCTAssertTrue(refreshed)
        XCTAssertEqual(model.items.map(\.id), ["fresh", "old"])
        let paths = await transport.sent.compactMap(\.path)
        XCTAssertEqual(paths.count, 4)
        XCTAssertEqual(paths.filter { $0.contains("scope=others") }.count, 2)
    }

    private var others: [Components.Schemas.FeedItem] {
        Array(Components.Schemas.FeedItem.publicProposals)
    }

    private func item(_ id: String, daysAgo: Double) -> Components.Schemas.FeedItem {
        var item = Components.Schemas.FeedItem.samples[0]
        item.id = id
        item.createdAt = now.addingTimeInterval(-daysAgo * day)
        item.updatedAt = item.createdAt
        return item
    }

    private func loaded(
        _ script: [StubTransport.Reply], hints: FakeHintStream = FakeHintStream()
    ) async throws -> (FeedModel, StubTransport) {
        let transport = StubTransport(scripted: script)
        let model = FeedModel(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            viewerID: nil,
            hints: hints,
            clock: TestClock(),
            now: { [now] in now }
        )
        await model.load()
        return (model, transport)
    }

    private func waitUntil(_ predicate: @escaping () async -> Bool) async -> Bool {
        let deadline = ContinuousClock.now.advanced(by: .seconds(5))
        while ContinuousClock.now < deadline {
            if await predicate() { return true }
            await Task.yield()
        }
        return await predicate()
    }

    private static func page(_ items: [Components.Schemas.FeedItem]) throws -> StubTransport.Reply {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        let page = Components.Schemas.FeedPage(items: items, nextCursor: nil)
        let data = try encoder.encode(page)
        return .json(.ok, String(decoding: data, as: UTF8.self))
    }
}
