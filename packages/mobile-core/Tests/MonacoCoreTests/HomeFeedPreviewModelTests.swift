import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestClock
import MonacoTestSupport
import XCTest

@MainActor
final class HomeFeedPreviewModelTests: XCTestCase {
    private let samples = Components.Schemas.FeedItem.samples

    func testLoadAsksForFiveOfTheMemberCabalsInOneGet() async throws {
        let (model, transport, _) = try makeModel(.yourCabals, [.json(.ok, try Self.page(samples))])
        await model.load()
        guard case .loaded(let items) = model.state else { return XCTFail("expected loaded, got \(model.state)") }
        XCTAssertEqual(items.count, samples.count)
        let sent = await queries(transport)
        XCTAssertEqual(sent.count, 1)
        XCTAssertEqual(sent[0]["scope"], "mine")
        XCTAssertEqual(sent[0]["limit"], "5")
        XCTAssertEqual(sent[0]["kind"], FeedQuery.shownKinds)
    }

    func testEveryoneAsksForFourOfTheOtherCabals() async throws {
        let (model, transport, _) = try makeModel(.everyone, [.json(.ok, try Self.page(samples))])
        await model.load()
        let sent = await queries(transport)
        XCTAssertEqual(sent[0]["scope"], "others")
        XCTAssertEqual(sent[0]["limit"], "4")
    }

    func testAFailedFirstLoadIsTheFailedState() async throws {
        let (model, _, _) = try makeModel(.yourCabals, [.failure(URLError(.notConnectedToInternet))])
        await model.load()
        guard case .failed = model.state else { return XCTFail("expected failed, got \(model.state)") }
    }

    func testAFailedReloadKeepsTheRowsAlreadyShown() async throws {
        let (model, _, _) = try makeModel(
            .yourCabals, [.json(.ok, try Self.page(samples)), .failure(URLError(.notConnectedToInternet))])
        await model.load()
        await model.load()
        guard case .loaded(let items) = model.state else { return XCTFail("expected loaded, got \(model.state)") }
        XCTAssertEqual(items.count, samples.count)
    }

    func testAFeedHintReloadsOnce() async throws {
        let (model, transport, hints) = try makeModel(
            .yourCabals, [.json(.ok, try Self.page([samples[0]])), .json(.ok, try Self.page(samples))])
        await model.load()
        try await startObserving(model, hints)
        await hints.send(.changed(.global, what: "feed", id: "1"))
        let reloaded = await waitUntil {
            if case .loaded(let items) = model.state { return items.count == self.samples.count }
            return false
        }
        XCTAssertTrue(reloaded)
        let sent = await transport.sent.count
        XCTAssertEqual(sent, 2)
    }

    func testAHintWhileHomeIsHiddenWaitsForItToComeBack() async throws {
        let (model, transport, hints) = try makeModel(
            .yourCabals, [.json(.ok, try Self.page([samples[0]])), .json(.ok, try Self.page(samples))])
        await model.load()
        try await startObserving(model, hints)
        model.setVisible(false)
        await hints.send(.changed(.global, what: "feed", id: "1"))
        for _ in 0..<50 { await Task.yield() }
        let whileHidden = await transport.sent.count
        XCTAssertEqual(whileHidden, 1)
        model.setVisible(true)
        let reloaded = await waitUntil { await transport.sent.count == 2 }
        XCTAssertTrue(reloaded)
    }

    private func startObserving(_ model: HomeFeedPreviewModel, _ hints: FakeHintStream) async throws {
        let observer = Task { await model.observe() }
        addTeardownBlock { observer.cancel() }
        let subscribed = await waitUntil { await hints.subscriberCount == 1 }
        XCTAssertTrue(subscribed)
    }

    private func makeModel(
        _ source: HomeFeedPreviewModel.Source, _ script: [StubTransport.Reply]
    ) throws -> (HomeFeedPreviewModel, StubTransport, FakeHintStream) {
        let transport = StubTransport(scripted: script)
        let hints = FakeHintStream()
        let model = HomeFeedPreviewModel(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            hints: hints, clock: TestClock(), source: source)
        return (model, transport, hints)
    }

    private func queries(_ transport: StubTransport) async -> [[String: String]] {
        await transport.sent.map { request in
            let items = URLComponents(string: request.path ?? "")?.queryItems ?? []
            return Dictionary(items.map { ($0.name, $0.value ?? "") }, uniquingKeysWith: { _, last in last })
        }
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
