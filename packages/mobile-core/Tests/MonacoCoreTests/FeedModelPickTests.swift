import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestClock
import MonacoTestSupport
import XCTest

@MainActor
final class FeedModelPickTests: XCTestCase {
    private let samples = Components.Schemas.FeedItem.samples

    func testAChipKeepsTheLoadedRowsUntilTheNewPageArrives() async throws {
        let transport = StubTransport(scripted: [.json(.ok, try Self.page(samples)), .gate])
        let model = makeModel(transport)
        await model.load()
        let pick = Task { await model.select(.proposals) }
        await transport.waitForRequests(2)
        XCTAssertEqual(model.items.count, samples.count)
        XCTAssertEqual(model.phase, .loaded)
        XCTAssertEqual(model.query.chip, .proposals)
        await transport.releaseGate(.json(.ok, try Self.page([samples[0]])))
        await pick.value
        XCTAssertEqual(model.items.map(\.id), [samples[0].id])
    }

    func testAScopeAndASearchKeepTheLoadedRowsToo() async throws {
        let scoped = StubTransport(scripted: [.json(.ok, try Self.page(samples)), .gate])
        let scopedModel = makeModel(scoped)
        await scopedModel.load()
        let scopePick = Task { await scopedModel.select(.following) }
        await scoped.waitForRequests(2)
        XCTAssertEqual(scopedModel.items.count, samples.count)
        XCTAssertEqual(scopedModel.phase, .loaded)
        XCTAssertEqual(scopedModel.query.scope, .following)
        await scoped.releaseGate(.json(.ok, try Self.page([samples[0]])))
        await scopePick.value
        XCTAssertEqual(scopedModel.items.map(\.id), [samples[0].id])

        let searched = StubTransport(scripted: [.json(.ok, try Self.page(samples)), .gate])
        let clock = TestClock()
        let searchModel = makeModel(searched, clock: clock)
        await searchModel.load()
        searchModel.setSearch("zz")
        _ = await clock.state.until { $0.pending == 1 }
        clock.advance(by: FeedModel.searchDebounce)
        await searched.waitForRequests(2)
        XCTAssertEqual(searchModel.items.count, samples.count)
        XCTAssertEqual(searchModel.phase, .loaded)
        XCTAssertEqual(searchModel.query.search, "zz")
        await searched.releaseGate(.json(.ok, try Self.page([samples[0]])))
        let expected = [samples[0].id]
        let replaced = await waitUntil { searchModel.items.map(\.id) == expected }
        XCTAssertTrue(replaced, "\(searchModel.items.count) rows")
    }

    func testAnOlderPickDoesNotReplaceANewerOne() async throws {
        let transport = StubTransport(scripted: [.json(.ok, try Self.page(samples)), .gate, .gate])
        let model = makeModel(transport)
        await model.load()
        let proposals = Task { await model.select(.proposals) }
        await transport.waitForRequests(2)
        let trades = Task { await model.select(.trades) }
        await transport.waitForRequests(3)
        await transport.releaseGate(.json(.ok, try Self.page([samples[0]])))
        await proposals.value
        XCTAssertEqual(model.items.count, samples.count)
        XCTAssertEqual(model.query.chip, .trades)
        await transport.releaseGate(.json(.ok, try Self.page([samples[1]])))
        await trades.value
        XCTAssertEqual(model.items.map(\.id), [samples[1].id])
    }

    func testAPickThatFailsShowsTheFailedStateNotTheOldRows() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.page(samples)),
            .failure(URLError(.notConnectedToInternet)),
        ])
        let model = makeModel(transport)
        await model.load()
        await model.select(.trades)
        guard case .failed = model.phase else { return XCTFail("expected failure, got \(model.phase)") }
        XCTAssertTrue(model.items.isEmpty)
    }

    func testAPickWithNothingOnScreenStillShowsTheSkeleton() async throws {
        let transport = StubTransport(scripted: [.json(.ok, try Self.page([])), .gate])
        let model = makeModel(transport)
        await model.load()
        XCTAssertEqual(model.phase, .empty(query: nil))
        let pick = Task { await model.select(.trades) }
        await transport.waitForRequests(2)
        XCTAssertEqual(model.phase, .loading)
        await transport.releaseGate(.json(.ok, try Self.page([])))
        await pick.value
    }

    func testTypingAgainWhileASearchLoadsKeepsTheLoadedRows() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.page(samples)), .hang, .json(.ok, try Self.page([samples[0]])),
        ])
        let clock = TestClock()
        let model = makeModel(transport, clock: clock)
        await model.load()
        await search("zz", in: model, clock: clock, transport: transport, requests: 2)
        model.setSearch("zzz")
        await settle()
        XCTAssertEqual(model.items.count, samples.count)
        XCTAssertEqual(model.phase, .loaded)
        _ = await clock.state.until { $0.pending == 1 }
        clock.advance(by: FeedModel.searchDebounce)
        let expected = [samples[0].id]
        let replaced = await waitUntil { model.items.map(\.id) == expected }
        XCTAssertTrue(replaced, "\(model.phase)")
        XCTAssertEqual(model.query.search, "zzz")
    }

    func testTappingTheSelectedChipWhileASearchLoadsKeepsTheLoadedRows() async throws {
        let transport = StubTransport(scripted: [.json(.ok, try Self.page(samples)), .hang, .gate])
        let clock = TestClock()
        let model = makeModel(transport, clock: clock)
        await model.load()
        await search("zz", in: model, clock: clock, transport: transport, requests: 2)
        let tap = Task { await model.select(.all) }
        await transport.waitForRequests(3)
        await settle()
        XCTAssertEqual(model.items.count, samples.count)
        XCTAssertEqual(model.phase, .loaded)
        await transport.releaseGate(.json(.ok, try Self.page([samples[0]])))
        await tap.value
        XCTAssertEqual(model.items.map(\.id), [samples[0].id])
    }

    func testATypedSpaceWhileASearchLoadsStillShowsThatSearch() async throws {
        let transport = StubTransport(scripted: [.json(.ok, try Self.page(samples)), .gate])
        let clock = TestClock()
        let model = makeModel(transport, clock: clock)
        await model.load()
        await search("zz", in: model, clock: clock, transport: transport, requests: 2)
        model.setSearch("zz ")
        await settle()
        await transport.releaseGate(.json(.ok, try Self.page([samples[0]])))
        let expected = [samples[0].id]
        let landed = await waitUntil { model.items.map(\.id) == expected }
        XCTAssertTrue(landed, "\(model.phase)")
        XCTAssertEqual(model.query.search, "zz")
    }

    func testTypingAgainWhileASearchLoadsOnAnEmptyFeedKeepsTheSkeleton() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.page([])), .hang, .json(.ok, try Self.page([samples[0]])),
        ])
        let clock = TestClock()
        let model = makeModel(transport, clock: clock)
        await model.load()
        await search("zz", in: model, clock: clock, transport: transport, requests: 2)
        XCTAssertEqual(model.phase, .loading)
        model.setSearch("zzz")
        await settle()
        XCTAssertEqual(model.phase, .loading)
        _ = await clock.state.until { $0.pending == 1 }
        clock.advance(by: FeedModel.searchDebounce)
        let expected = [samples[0].id]
        let replaced = await waitUntil { model.items.map(\.id) == expected }
        XCTAssertTrue(replaced, "\(model.phase)")
    }

    private func search(
        _ text: String, in model: FeedModel, clock: TestClock, transport: StubTransport, requests: Int
    ) async {
        model.setSearch(text)
        _ = await clock.state.until { $0.pending == 1 }
        clock.advance(by: FeedModel.searchDebounce)
        await transport.waitForRequests(requests)
    }

    private func settle() async {
        for _ in 0..<200 { await Task.yield() }
    }

    private func makeModel(_ transport: StubTransport, clock: any Clock<Duration> = TestClock()) -> FeedModel {
        FeedModel(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            viewerID: nil,
            hints: FakeHintStream(),
            clock: clock
        )
    }

    private func waitUntil(_ predicate: @escaping () async -> Bool) async -> Bool {
        let deadline = ContinuousClock.now.advanced(by: .seconds(5))
        while ContinuousClock.now < deadline {
            if await predicate() { return true }
            await Task.yield()
        }
        return await predicate()
    }

    private static func page(_ items: [Components.Schemas.FeedItem]) throws -> String {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        let page = Components.Schemas.FeedPage(items: items, nextCursor: nil)
        return String(decoding: try encoder.encode(page), as: UTF8.self)
    }
}
