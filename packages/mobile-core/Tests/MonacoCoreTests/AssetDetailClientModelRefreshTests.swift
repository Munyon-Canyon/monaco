import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class AssetDetailClientModelRefreshTests: XCTestCase {
    func testPricesUpdatedRefetchesTheDetail() async throws {
        let (model, hints, transport) = try model(with: [.detail, .day, .detail, .day])
        await model.load()
        let observer = Task { await model.observe() }
        addTeardownBlock { observer.cancel() }
        let subscribed = await waitUntil { await hints.subscriberCount == 2 }
        XCTAssertTrue(subscribed)

        await hints.send(.changed(.global, what: "prices_updated", id: "1"))

        let refreshed = await waitUntil { await self.assetRequests(transport) == 4 }
        XCTAssertTrue(refreshed)
    }

    func testPriceHintDoesNotRefreshAWeekChart() async throws {
        let (model, hints, transport) = try model(with: [.detail, .day, .week, .detail])
        await model.load()
        await model.loadChart(range: .oneWeek)
        let observer = Task { await model.observe() }
        addTeardownBlock { observer.cancel() }
        _ = await waitUntil { await hints.subscriberCount == 2 }

        await hints.send(.changed(.global, what: "prices_updated", id: "1"))

        let refreshed = await waitUntil { await self.assetRequests(transport) == 4 }
        XCTAssertTrue(refreshed)
        XCTAssertEqual(model.selectedRange, .oneWeek)
    }

    func testResyncRefetchesTheDayDetailAndChart() async throws {
        let (model, hints, transport) = try model(with: [.detail, .day, .detail, .day])
        await model.load()
        let observer = Task { await model.observe() }
        addTeardownBlock { observer.cancel() }
        _ = await waitUntil { await hints.subscriberCount == 2 }

        await hints.send(.resync)

        let refreshed = await waitUntil { await self.assetRequests(transport) == 4 }
        XCTAssertTrue(refreshed)
    }

    func testOtherGlobalHintDoesNotRefetch() async throws {
        let (model, hints, transport) = try model(with: [.detail, .day])
        await model.load()
        let observer = Task { await model.observe() }
        addTeardownBlock { observer.cancel() }
        _ = await waitUntil { await hints.subscriberCount == 2 }

        await hints.send(.changed(.global, what: "other", id: "1"))
        for _ in 0..<10 { await Task.yield() }

        let requestCount = await assetRequests(transport)
        XCTAssertEqual(requestCount, 2)
    }

    func testHiddenScreenDefersTheRefresh() async throws {
        let (model, hints, transport) = try model(with: [.detail, .day, .detail, .day])
        await model.load()
        let observer = Task { await model.observe() }
        addTeardownBlock { observer.cancel() }
        _ = await waitUntil { await hints.subscriberCount == 2 }
        model.setVisible(false)

        await hints.send(.changed(.global, what: "prices_updated", id: "1"))
        for _ in 0..<10 { await Task.yield() }
        let deferredRequestCount = await assetRequests(transport)
        XCTAssertEqual(deferredRequestCount, 2)

        model.setVisible(true)
        let refreshed = await waitUntil { await self.assetRequests(transport) == 4 }
        XCTAssertTrue(refreshed)
    }

    private enum Reply {
        case detail, day, week
    }

    private func model(with replies: [Reply]) throws -> (AssetDetailClientModel, FakeHintStream, StubTransport) {
        let hints = FakeHintStream()
        var routes: [String: [StubTransport.Reply]] = [:]
        for kind in replies {
            routes[path(kind), default: []].append(try reply(kind))
        }
        let transport = StubTransport(routes: routes)
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        return (AssetDetailClientModel(api: api, symbol: "AAPLx", hints: hints), hints, transport)
    }

    private func path(_ reply: Reply) -> String {
        switch reply {
        case .detail: "/v1/assets/AAPLx"
        case .day: "/v1/assets/AAPLx/chart?range=1D"
        case .week: "/v1/assets/AAPLx/chart?range=1W"
        }
    }

    private func assetRequests(_ transport: StubTransport) async -> Int {
        await transport.sent.compactMap(\.path).filter { $0.hasPrefix("/v1/assets/") }.count
    }

    private func reply(_ reply: Reply) throws -> StubTransport.Reply {
        let value: any Encodable =
            switch reply {
            case .detail: Components.Schemas.AssetDetail.googl
            case .day: Components.Schemas.AssetChart.oneDay
            case .week:
                Components.Schemas.AssetChart(
                    range: ._1w, interval: ._5m, bucketSeconds: 300, points: [], empty: true, attribution: "Test",
                    nextBefore: nil)
            }
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return .json(.ok, String(decoding: try encoder.encode(value), as: UTF8.self))
    }

    private func waitUntil(_ predicate: @escaping () async -> Bool) async -> Bool {
        for _ in 0..<500 {
            if await predicate() { return true }
            await Task.yield()
        }
        return await predicate()
    }
}
