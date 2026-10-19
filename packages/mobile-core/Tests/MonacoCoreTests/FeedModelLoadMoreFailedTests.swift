import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestClock
import MonacoTestSupport
import XCTest

@MainActor
final class FeedModelLoadMoreFailedTests: XCTestCase {
    private let samples = Components.Schemas.FeedItem.samples

    func testAFailedNextPageIsLoadMoreFailedUntilARetrySucceeds() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.page(Array(samples.prefix(2)), next: "c2")),
            .failure(URLError(.notConnectedToInternet)),
            .failure(URLError(.notConnectedToInternet)),
            .json(.ok, try Self.page(Array(samples.suffix(3)))),
        ])
        let model = makeModel(transport)
        await model.load()
        XCTAssertFalse(model.loadMoreFailed)
        await model.loadMore()
        XCTAssertTrue(model.loadMoreFailed)
        await model.loadMore()
        XCTAssertTrue(model.loadMoreFailed, "a retry that fails again keeps the failure")
        await model.loadMore()
        XCTAssertFalse(model.loadMoreFailed)
        XCTAssertEqual(model.items.count, 5)
    }

    func testAFailedRefreshIsNotALoadMoreFailure() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.page(Array(samples.prefix(2)), next: "c2")),
            .failure(URLError(.notConnectedToInternet)),
        ])
        let model = makeModel(transport)
        await model.load()
        await model.refresh()
        XCTAssertEqual(model.failureTick, 1)
        XCTAssertFalse(model.loadMoreFailed)
    }

    func testAFailedNextPageRaisesNoToast() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.page(Array(samples.prefix(2)), next: "c2")),
            .failure(URLError(.notConnectedToInternet)),
        ])
        let model = makeModel(transport)
        await model.load()
        await model.loadMore()
        XCTAssertTrue(model.loadMoreFailed)
        XCTAssertEqual(model.failureTick, 0, "the inline notice shows a failed next page, not a toast")
    }

    func testAFailedMuteStillRaisesAToastWhileTheNextPageNoticeShows() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.page(Array(samples.prefix(2)), next: "c2")),
            .failure(URLError(.notConnectedToInternet)),
            .failure(URLError(.notConnectedToInternet)),
        ])
        let model = makeModel(transport)
        await model.load()
        await model.loadMore()
        XCTAssertTrue(model.loadMoreFailed)
        let receipt = await model.mute(.hide(samples[0]))
        XCTAssertNil(receipt)
        XCTAssertEqual(model.failureTick, 1)
        XCTAssertNotNil(model.lastError)
    }

    func testChangingTheFilterClearsALoadMoreFailure() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, try Self.page(Array(samples.prefix(2)), next: "c2")),
            .failure(URLError(.notConnectedToInternet)),
            .json(.ok, try Self.page([samples[1]])),
        ])
        let model = makeModel(transport)
        await model.load()
        await model.loadMore()
        XCTAssertTrue(model.loadMoreFailed)
        await model.toggle(.completedTrades)
        XCTAssertFalse(model.loadMoreFailed)
    }

    private func makeModel(_ transport: StubTransport) -> FeedModel {
        FeedModel(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            viewerID: nil, hints: FakeHintStream(), clock: TestClock())
    }

    private static func page(_ items: [Components.Schemas.FeedItem], next: String? = nil) throws -> String {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        let page = Components.Schemas.FeedPage(items: items, nextCursor: next)
        return String(decoding: try encoder.encode(page), as: UTF8.self)
    }
}
