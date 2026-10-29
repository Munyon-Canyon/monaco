import XCTest
import os

@testable import Monaco

nonisolated final class GifDataCacheTests: XCTestCase {
    private var directory = URL(filePath: NSTemporaryDirectory())

    override func setUp() {
        super.setUp()
        GifStubProtocol.reset()
        directory = URL(filePath: NSTemporaryDirectory()).appending(path: "gif-cache-\(UUID().uuidString)")
    }

    override func tearDown() {
        try? FileManager.default.removeItem(at: directory)
        super.tearDown()
    }

    private func cache(maxBytes: Int = 1 << 20) -> GifDataCache {
        GifDataCache(session: GifStubProtocol.session(), maxBytes: maxBytes, directory: directory)
    }

    private func remote(_ name: String, bytes: Int = 100, status: Int = 200) throws -> URL {
        let url = try XCTUnwrap(URL(string: "https://gifs.test/\(name).gif"))
        GifStubProtocol.respond(to: url, status: status, body: Data(repeating: 7, count: bytes))
        return url
    }

    @MainActor
    func testTwoConcurrentCallersShareOneDownload() async throws {
        let cache = cache()
        let url = try remote("dance")

        async let first = cache.fileURL(for: url)
        async let second = cache.fileURL(for: url)
        let files = try await [first, second]

        XCTAssertEqual(files[0], files[1])
        XCTAssertEqual(GifStubProtocol.requestCount(for: url), 1)
        XCTAssertEqual(try Data(contentsOf: files[0]).count, 100)
        _ = try await cache.fileURL(for: url)
        XCTAssertEqual(GifStubProtocol.requestCount(for: url), 1)
    }

    @MainActor
    func testPastTheBudgetTheOldestFileIsRemoved() async throws {
        let cache = cache(maxBytes: 250)
        let first = try await cache.fileURL(for: try remote("a"))
        let second = try await cache.fileURL(for: try remote("b"))
        let third = try await cache.fileURL(for: try remote("c"))

        XCTAssertFalse(FileManager.default.fileExists(atPath: first.path))
        XCTAssertTrue(FileManager.default.fileExists(atPath: second.path))
        XCTAssertTrue(FileManager.default.fileExists(atPath: third.path))
        let total = await cache.totalBytes()
        XCTAssertEqual(total, 200)
    }

    @MainActor
    func testAFileUsedAgainOutlivesAnOlderOne() async throws {
        let cache = cache(maxBytes: 250)
        let a = try remote("a")
        let first = try await cache.fileURL(for: a)
        let second = try await cache.fileURL(for: try remote("b"))
        _ = try await cache.fileURL(for: a)
        _ = try await cache.fileURL(for: try remote("c"))

        XCTAssertTrue(FileManager.default.fileExists(atPath: first.path))
        XCTAssertFalse(FileManager.default.fileExists(atPath: second.path))
    }

    @MainActor
    func testAFailedDownloadThrowsAndCachesNothing() async throws {
        let cache = cache()
        let url = try remote("broken", status: 500)

        do {
            _ = try await cache.fileURL(for: url)
            XCTFail("a 500 must throw")
        } catch {}

        let total = await cache.totalBytes()
        XCTAssertEqual(total, 0)
        let files = (try? FileManager.default.contentsOfDirectory(atPath: directory.path)) ?? []
        XCTAssertTrue(files.isEmpty)

        GifStubProtocol.respond(to: url, status: 200, body: Data(repeating: 1, count: 10))
        _ = try await cache.fileURL(for: url)
        XCTAssertEqual(GifStubProtocol.requestCount(for: url), 2)
    }
}

nonisolated private final class GifStubProtocol: URLProtocol {
    private struct State {
        var stubs: [URL: (status: Int, body: Data)] = [:]
        var counts: [URL: Int] = [:]
    }

    private static let state = OSAllocatedUnfairLock(initialState: State())

    nonisolated static func session() -> URLSession {
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [GifStubProtocol.self]
        return URLSession(configuration: configuration)
    }

    nonisolated static func reset() {
        state.withLock { $0 = State() }
    }

    nonisolated static func respond(to url: URL, status: Int, body: Data) {
        state.withLock { $0.stubs[url] = (status, body) }
    }

    nonisolated static func requestCount(for url: URL) -> Int {
        state.withLock { $0.counts[url] ?? 0 }
    }

    nonisolated override class func canInit(with request: URLRequest) -> Bool { true }
    nonisolated override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }

    nonisolated override func startLoading() {
        guard let url = request.url else { return }
        let stub = Self.state.withLock { state -> (status: Int, body: Data)? in
            state.counts[url, default: 0] += 1
            return state.stubs[url]
        }
        let status = stub?.status ?? 404
        guard let response = HTTPURLResponse(url: url, statusCode: status, httpVersion: nil, headerFields: nil) else {
            return
        }
        client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
        client?.urlProtocol(self, didLoad: stub?.body ?? Data())
        client?.urlProtocolDidFinishLoading(self)
    }

    nonisolated override func stopLoading() {}
}
