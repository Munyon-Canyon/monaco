import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestClock
import MonacoTestSupport
import XCTest

@MainActor
final class FeedModelPickTests: XCTestCase {
    private let samples = Components.Schemas.FeedItem.samples

    func testAScopeKeepsTheLoadedRowsUntilTheNewPageArrives() async throws {
        let transport = StubTransport(scripted: [.json(.ok, try Self.page(samples)), .gate])
        let model = makeModel(transport)
        await model.load()
        let pick = Task { await model.select(.following) }
        await transport.waitForRequests(2)
        XCTAssertEqual(model.items.count, samples.count)
        XCTAssertEqual(model.phase, .loaded)
        XCTAssertEqual(model.query.scope, .following)
        await transport.releaseGate(.json(.ok, try Self.page([samples[0]])))
        await pick.value
        XCTAssertEqual(model.items.map(\.id), [samples[0].id])
    }

    func testAnOlderPickDoesNotReplaceANewerOne() async throws {
        let transport = StubTransport(scripted: [.json(.ok, try Self.page(samples)), .gate, .gate])
        let model = makeModel(transport)
        await model.load()
        let mine = Task { await model.select(.mine) }
        await transport.waitForRequests(2)
        let everyone = Task { await model.select(.everyone) }
        await transport.waitForRequests(3)
        await transport.releaseGate(.json(.ok, try Self.page([samples[0]])))
        await mine.value
        XCTAssertEqual(model.items.count, samples.count)
        XCTAssertEqual(model.query.scope, .everyone)
        await transport.releaseGate(.json(.ok, try Self.page([samples[1]])))
        await everyone.value
        XCTAssertEqual(model.items.map(\.id), [samples[1].id])
    }

    func testAPickThatFailsShowsTheFailedStateNotTheOldRows() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.page(samples)),
            .failure(URLError(.notConnectedToInternet)),
        ])
        let model = makeModel(transport)
        await model.load()
        await model.select(.everyone)
        guard case .failed = model.phase else { return XCTFail("expected failure, got \(model.phase)") }
        XCTAssertTrue(model.items.isEmpty)
    }

    func testAPickWithNothingOnScreenStillShowsTheSkeleton() async throws {
        let transport = StubTransport(scripted: [.json(.ok, try Self.page([])), .gate])
        let model = makeModel(transport)
        await model.load()
        XCTAssertEqual(model.phase, .empty)
        let pick = Task { await model.select(.everyone) }
        await transport.waitForRequests(2)
        XCTAssertEqual(model.phase, .loading)
        await transport.releaseGate(.json(.ok, try Self.page([])))
        await pick.value
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
