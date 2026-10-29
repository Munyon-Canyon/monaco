import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestClock
import XCTest

@MainActor
final class GifSearchModelTests: XCTestCase {
    func testLoadShowsTrendingWithTheAttribution() async {
        let model = GifSearchModel(gifs: SampleGifProvider(), clock: TestClock())
        model.load()
        let loaded = await waitUntil { model.phase == .loaded(SampleGifProvider.items) }
        XCTAssertTrue(loaded)
        XCTAssertEqual(model.attribution, "Powered by GIPHY")
    }

    func testTypingWithinTheDebounceSearchesOnceForTheLastQuery() async {
        let provider = RecordingGifProvider()
        let clock = TestClock()
        let model = GifSearchModel(gifs: provider, clock: clock)
        model.query = "r"
        model.query = "ro"
        model.query = "rocket"
        XCTAssertEqual(model.phase, .loading)
        let parked = await clock.state.until { $0.pending == 1 }
        XCTAssertTrue(parked)
        XCTAssertEqual(clock.state.current.requested.last, GifSearchModel.debounce)
        clock.advance(by: GifSearchModel.debounce)
        let loaded = await waitUntil { model.phase != .loading }
        XCTAssertTrue(loaded)
        let calls = await provider.log.calls
        XCTAssertEqual(calls, ["search rocket 0"])
    }

    func testAnEmptyQueryShowsTrendingAgain() async {
        let provider = RecordingGifProvider()
        let clock = TestClock()
        let model = GifSearchModel(gifs: provider, clock: clock)
        model.query = "rocket"
        _ = await clock.state.until { $0.pending == 1 }
        clock.advance(by: GifSearchModel.debounce)
        _ = await waitUntil { await provider.log.calls.count == 1 }
        model.query = "  "
        _ = await clock.state.until { $0.pending == 1 }
        clock.advance(by: GifSearchModel.debounce)
        _ = await waitUntil { await provider.log.calls.count == 2 }
        let calls = await provider.log.calls
        XCTAssertEqual(calls, ["search rocket 0", "trending 0"])
    }

    func testLoadMorePagesByTheNextOffsetUntilItRunsOut() async {
        let provider = RecordingGifProvider(pages: 3)
        let model = GifSearchModel(gifs: provider, clock: TestClock())
        model.load()
        _ = await waitUntil { model.phase != .loading }
        XCTAssertTrue(model.canLoadMore)
        model.loadMore()
        _ = await waitUntil { model.canLoadMore }
        model.loadMore()
        let done = await waitUntil { !model.canLoadMore && self.count(model) == 6 }
        XCTAssertTrue(done)
        model.loadMore()
        let calls = await provider.log.calls
        XCTAssertEqual(calls, ["trending 0", "trending 2", "trending 4"])
    }

    func testAFailingProviderFailsTheSheetAndRetryReloads() async {
        let model = GifSearchModel(gifs: SampleGifProvider(mode: .failing), clock: TestClock())
        model.load()
        let failed = await waitUntil {
            if case .failed = model.phase { return true }
            return false
        }
        XCTAssertTrue(failed)
        XCTAssertEqual(GifSearchModel.unavailableCopy, "GIFs aren’t available right now.")
        model.retry()
        XCTAssertEqual(model.phase, .loading)
    }

    func testAnUnavailableProviderFailsTheSheet() async {
        let model = GifSearchModel(gifs: UnavailableGifProvider(), clock: TestClock())
        model.load()
        let failed = await waitUntil {
            if case .failed = model.phase { return true }
            return false
        }
        XCTAssertTrue(failed)
    }

    func testANoMatchSearchLoadsAnEmptyList() async {
        let clock = TestClock()
        let model = GifSearchModel(gifs: SampleGifProvider(mode: .noMatches), clock: clock)
        model.query = "zzzz"
        _ = await clock.state.until { $0.pending == 1 }
        clock.advance(by: GifSearchModel.debounce)
        let empty = await waitUntil { model.phase == .loaded([]) }
        XCTAssertTrue(empty)
    }

    func testSelectBuildsAGifAttachmentWithAllThreeMedia() async throws {
        let model = GifSearchModel(gifs: SampleGifProvider(), clock: TestClock())
        let gif = try XCTUnwrap(SampleGifProvider.items.first)
        guard case .gif(let attachment) = model.select(gif) else { return XCTFail("not a gif attachment") }
        XCTAssertEqual(attachment.kind, .gif)
        XCTAssertEqual(attachment.gif.id, gif.id)
        XCTAssertEqual(attachment.gif.title, gif.title)
        XCTAssertEqual(attachment.gif.still.url, gif.still.url.absoluteString)
        XCTAssertEqual(attachment.gif.preview.url, gif.preview.url.absoluteString)
        XCTAssertEqual(attachment.gif.original.url, gif.original.url.absoluteString)
        XCTAssertEqual(attachment.gif.original.width, gif.original.width)
        XCTAssertEqual(attachment.gif.original.height, gif.original.height)
    }

    private func count(_ model: GifSearchModel) -> Int {
        if case .loaded(let items) = model.phase { return items.count }
        return 0
    }

    private func waitUntil(_ predicate: @escaping () async -> Bool) async -> Bool {
        for _ in 0..<500 {
            if await predicate() { return true }
            await Task.yield()
        }
        return await predicate()
    }
}

private actor CallLog {
    private(set) var calls: [String] = []

    func record(_ call: String) {
        calls.append(call)
    }
}

private struct RecordingGifProvider: GifProvider {
    let log = CallLog()
    var pages = 1

    func trending(offset: Int) async throws -> GifPage {
        await log.record("trending \(offset)")
        return page(offset)
    }

    func search(_ query: String, offset: Int) async throws -> GifPage {
        await log.record("search \(query) \(offset)")
        return page(offset)
    }

    private func page(_ offset: Int) -> GifPage {
        let items = SampleGifProvider.items.dropFirst(offset).prefix(2)
        return GifPage(items: Array(items), nextOffset: offset / 2 + 1 < pages ? offset + 2 : nil)
    }
}
