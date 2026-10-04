import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestClock
import MonacoTestSupport
import XCTest

@MainActor
final class FeedModelTests: XCTestCase {
    private let samples = Components.Schemas.FeedItem.samples

    func testAChipBuildsANewPagerThatSendsItsKind() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.page(samples)),
            .json(.ok, try Self.page([samples[0]])),
        ])
        let model = makeModel(transport)
        await model.load()
        let first = model.pager
        XCTAssertEqual(model.items.count, samples.count)
        await model.select(.proposals)
        XCTAssertFalse(model.pager === first)
        XCTAssertEqual(model.items.map(\.id), [samples[0].id])
        let sent = await queries(transport)
        XCTAssertNil(sent[0]["kind"] ?? nil)
        XCTAssertEqual(sent[0]["scope"], "all")
        XCTAssertEqual(sent[1]["kind"], "proposal")
    }

    func testTheScopeKeepsTheChip() async throws {
        let transport = StubTransport(.json(.ok, try Self.page(samples)))
        let model = makeModel(transport)
        await model.select(.cabals)
        await model.select(.following)
        let last = await queries(transport).last
        XCTAssertEqual(last?["kind"], "cabal_created,member_joined")
        XCTAssertEqual(last?["scope"], "following")
    }

    func testSearchWaitsForTheDebounceAndSendsOnlyTheLastText() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.page(samples)),
            .json(.ok, try Self.page([])),
        ])
        let clock = TestClock()
        let model = makeModel(transport, clock: clock)
        await model.load()
        model.setSearch("ts")
        model.setSearch("  zzqq ")
        _ = await clock.state.until { $0.pending == 1 }
        let beforeDebounce = await transport.sent.count
        XCTAssertEqual(beforeDebounce, 1)
        clock.advance(by: FeedModel.searchDebounce)
        let searched = await waitUntil { model.phase == .empty(query: "zzqq") }
        XCTAssertTrue(searched, "\(model.phase)")
        let sent = await queries(transport)
        XCTAssertEqual(sent.count, 2)
        XCTAssertEqual(sent[1]["q"], "zzqq")
    }

    func testAnEmptyFeedWithNoQueryNamesNoQuery() async throws {
        let model = makeModel(StubTransport(.json(.ok, try Self.page([]))))
        await model.load()
        XCTAssertEqual(model.phase, .empty(query: nil))
    }

    func testLoadMoreIsSingleFlight() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.page(Array(samples.prefix(2)), next: "c2")),
            .gate,
        ])
        let model = makeModel(transport)
        await model.load()
        let first = Task { await model.loadMore() }
        let asked = await waitUntil { await transport.sent.count == 2 }
        XCTAssertTrue(asked)
        await model.loadMore()
        XCTAssertTrue(model.isLoadingMore)
        await transport.releaseGate(.json(.ok, try Self.page(Array(samples.suffix(3)))))
        await first.value
        let sent = await queries(transport)
        XCTAssertEqual(sent.count, 2)
        XCTAssertEqual(sent[1]["cursor"], "c2")
        XCTAssertEqual(model.items.count, samples.count)
    }

    func testAFailedFirstPageIsTheFailedStateWithoutAToast() async {
        let model = makeModel(StubTransport(scripted: [.failure(URLError(.notConnectedToInternet))]))
        await model.load()
        guard case .failed(let error) = model.phase else { return XCTFail("expected failure, got \(model.phase)") }
        XCTAssertEqual(ToastCopy.message(for: error), "You're offline. Try again.")
        XCTAssertEqual(model.failureTick, 0)
    }

    func testAFailedNextPageKeepsTheItemsAndBumpsTheTick() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.page(Array(samples.prefix(2)), next: "c2")),
            .failure(URLError(.notConnectedToInternet)),
        ])
        let model = makeModel(transport)
        await model.load()
        await model.loadMore()
        XCTAssertEqual(model.phase, .loaded)
        XCTAssertEqual(model.items.count, 2)
        XCTAssertEqual(model.failureTick, 1)
        XCTAssertEqual(model.lastError.map(ToastCopy.message(for:)), "You're offline. Try again.")
    }

    private func makeModel(_ transport: StubTransport, clock: any Clock<Duration> = TestClock()) -> FeedModel {
        FeedModel(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            hints: FakeHintStream(),
            clock: clock
        )
    }

    private func queries(_ transport: StubTransport) async -> [[String: String]] {
        await transport.sent.map { request in
            let items = URLComponents(string: request.path ?? "")?.queryItems ?? []
            return Dictionary(items.map { ($0.name, $0.value ?? "") }, uniquingKeysWith: { _, last in last })
        }
    }

    private func waitUntil(_ predicate: @escaping () async -> Bool) async -> Bool {
        for _ in 0..<500 {
            if await predicate() { return true }
            await Task.yield()
        }
        return await predicate()
    }

    private static func page(_ items: [Components.Schemas.FeedItem], next: String? = nil) throws -> String {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        let page = Components.Schemas.FeedPage(items: items, nextCursor: next)
        return String(decoding: try encoder.encode(page), as: UTF8.self)
    }
}
